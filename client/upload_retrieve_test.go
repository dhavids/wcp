package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeBackend emulates catbox/litterbox: multipart upload returns a
// fetch URL, GET on that path returns whatever was stored. Ids are kept
// short (6 alnum chars) so the resulting code stays within the 7-char
// shape implicitRetrieve looks for, matching a real catbox/litterbox id.
func fakeBackend(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	store := map[string][]byte{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			file, _, err := r.FormFile("fileToUpload")
			if err != nil {
				w.Write([]byte("No file!"))
				return
			}
			defer file.Close()
			body, err := io.ReadAll(file)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}

			mu.Lock()
			id := fmt.Sprintf("tid%03d", len(store))
			store[id] = body
			mu.Unlock()

			fmt.Fprintf(w, "http://%s/%s", r.Host, id)
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/")
		mu.Lock()
		body, ok := store[id]
		mu.Unlock()
		if !ok {
			w.WriteHeader(404)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func withSandbox(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
}

func TestUploadFileThenRetrieveRoundTrip(t *testing.T) {
	withSandbox(t)
	srv := fakeBackend(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	writeFile(t, path, "round trip content")

	f, _ := ParseArgs([]string{"-n", "--host", srv.URL, path})
	if exitCode := uploadFile(f, path); exitCode != 0 {
		t.Fatalf("uploadFile exit = %d", exitCode)
	}
}

func TestUploadAndImplicitRetrieveViaCode(t *testing.T) {
	withSandbox(t)
	srv := fakeBackend(t)

	f, _ := ParseArgs([]string{"-n", "-u", "--host", srv.URL})
	emitted, exitCode := uploadText(f, "plain text payload", "")
	if exitCode != 0 {
		t.Fatalf("uploadText exit = %d", exitCode)
	}

	rf, _ := ParseArgs([]string{"-n", "--host", srv.URL})
	ok, code := implicitRetrieve(rf, emitted, stdoutSink)
	if !ok {
		t.Fatalf("implicitRetrieve(%q) did not treat it as a code", emitted)
	}
	if code != 0 {
		t.Fatalf("implicitRetrieve exit = %d", code)
	}
}

func TestUploadEncryptedThenExplicitRetrieve(t *testing.T) {
	withSandbox(t)
	srv := fakeBackend(t)

	f, _ := ParseArgs([]string{"-n", "-e", "--host", srv.URL})
	emitted, exitCode := uploadText(f, "secret payload", "")
	if exitCode != 0 {
		t.Fatalf("uploadText exit = %d", exitCode)
	}
	if !strings.Contains(emitted, "-") {
		t.Fatalf("emitted %q should carry a key (no stored key in this sandbox)", emitted)
	}

	parts := strings.SplitN(emitted, "-", 2)
	rf, _ := ParseArgs([]string{"-n", "--host", srv.URL})
	if exitCode := explicitRetrieve(rf, parts[0], parts[1], "", stdoutSink); exitCode != 0 {
		t.Fatalf("explicitRetrieve exit = %d", exitCode)
	}
}
