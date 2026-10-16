// Package backend talks to catbox and litterbox over HTTP, porting the
// bash/PowerShell clients' multipart upload and plain-GET retrieval
// exactly, including their curl timeout values.
package backend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"strings"
	"time"

	"wcp/utils/code"
)

const (
	// ConnectTimeout bounds TCP/TLS connection setup only.
	ConnectTimeout = 10 * time.Second
	// RetrieveMaxTime bounds a whole retrieval.
	RetrieveMaxTime = 30 * time.Second
	// UploadMaxTime bounds a whole upload — larger, since -f raises the
	// encrypted-upload ceiling to 100MB.
	UploadMaxTime = 300 * time.Second

	catboxUploadURL     = "https://catbox.moe/user/api.php"
	catboxFetchURL      = "https://files.catbox.moe"
	litterboxUploadURL  = "https://litterbox.catbox.moe/resources/internals/api.php"
	litterboxFetchURL   = "https://litter.catbox.moe"
	defaultLitterboxTTL = "1h"
)

// Options configures one upload.
type Options struct {
	Backend code.Backend
	// Host overrides the default upload endpoint for Backend.
	Host string
	// LitterboxTTL is litterbox's `time` field (e.g. "1h"); ignored for
	// catbox. Defaults to defaultLitterboxTTL when empty.
	LitterboxTTL string
}

// UploadError reports a backend reply that wasn't a URL — the reply text
// is collapsed the same way the bash client's describe_reply does, so an
// HTML error page doesn't flood the terminal.
type UploadError struct {
	Backend code.Backend
	Host    string
	Reply   string
}

func (e *UploadError) Error() string {
	return fmt.Sprintf("upload failed on backend %q (%s): backend reply: %s",
		e.Backend, e.Host, describeReply(e.Reply))
}

// Public functions

// UploadHost returns a backend's default upload endpoint.
func UploadHost(b code.Backend) string {
	if b == code.Catbox {
		return catboxUploadURL
	}
	return litterboxUploadURL
}

// FetchHost returns a backend's default retrieval endpoint.
func FetchHost(b code.Backend) string {
	if b == code.Catbox {
		return catboxFetchURL
	}
	return litterboxFetchURL
}

// Upload posts data as filename to the backend in opts, returning the
// bare id (e.g. "abc123.txt") the backend assigned.
func Upload(ctx context.Context, opts Options, filename string, data io.Reader) (string, error) {
	body, contentType, err := buildMultipart(opts, filename, data)
	if err != nil {
		return "", err
	}

	host := opts.Host
	if host == "" {
		host = UploadHost(opts.Backend)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, host, body)
	if err != nil {
		return "", fmt.Errorf("backend: building request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := client(UploadMaxTime).Do(req)
	if err != nil {
		return "", fmt.Errorf("backend: upload request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("backend: reading upload reply: %w", err)
	}

	reply := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(reply, "http://") && !strings.HasPrefix(reply, "https://") {
		return "", &UploadError{Backend: opts.Backend, Host: host, Reply: reply}
	}
	return stripHost(reply), nil
}

// Retrieve GETs url and returns the response body plus its
// Content-Type — a plain (unencrypted) retrieval needs that to decide
// whether to print the body or save it, since it carries no wcp-name
// header (that's only added when encrypting). Fails on any status 400
// and above.
func Retrieve(ctx context.Context, url string) (body []byte, contentType string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("backend: building request: %w", err)
	}

	resp, err := client(RetrieveMaxTime).Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("backend: retrieval request: %w", err)
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("backend: reading retrieval reply: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("backend: retrieval failed with status %d", resp.StatusCode)
	}
	return body, resp.Header.Get("Content-Type"), nil
}

// Helpers

func buildMultipart(opts Options, filename string, data io.Reader) (*bytes.Buffer, string, error) {
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)

	if err := w.WriteField("reqtype", "fileupload"); err != nil {
		return nil, "", fmt.Errorf("backend: %w", err)
	}
	if opts.Backend == code.Litterbox {
		ttl := opts.LitterboxTTL
		if ttl == "" {
			ttl = defaultLitterboxTTL
		}
		if err := w.WriteField("time", ttl); err != nil {
			return nil, "", fmt.Errorf("backend: %w", err)
		}
	}

	fw, err := w.CreateFormFile("fileToUpload", filename)
	if err != nil {
		return nil, "", fmt.Errorf("backend: %w", err)
	}
	if _, err := io.Copy(fw, data); err != nil {
		return nil, "", fmt.Errorf("backend: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("backend: %w", err)
	}
	return buf, w.FormDataContentType(), nil
}

// client builds an http.Client whose connect phase is capped by
// ConnectTimeout and whole-request time by maxTime — curl's
// --connect-timeout and --max-time map onto different knobs in net/http.
func client(maxTime time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: ConnectTimeout}
	return &http.Client{
		Timeout: maxTime,
		Transport: &http.Transport{
			DialContext: dialer.DialContext,
		},
	}
}

// stripHost matches the bash client's strip_host: drop the scheme and
// the first path segment (the host), keep everything after.
func stripHost(rawURL string) string {
	s := strings.TrimPrefix(rawURL, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.Index(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return ""
}

// describeReply collapses an HTML error page to one line and truncates
// long replies, matching the bash client's describe_reply.
func describeReply(reply string) string {
	if reply == "" {
		return "<empty response>"
	}
	lower := strings.ToLower(reply)
	if strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype") {
		out := "HTML error page"
		if title := between(reply, "<title>", "<"); title != "" {
			out += " - " + title
		}
		if reqID := between(reply, "Request ID: <code>", "<"); reqID != "" {
			out += fmt.Sprintf(" (request id %s)", reqID)
		}
		return out
	}
	if len(reply) > 200 {
		return fmt.Sprintf("%s... [%d bytes total]", reply[:200], len(reply))
	}
	return reply
}

func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	rest := s[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		return ""
	}
	return rest[:j]
}
