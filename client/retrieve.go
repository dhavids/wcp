package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"wcp/utils/backend"
	"wcp/utils/cipher"
	"wcp/utils/clipboard"
	"wcp/utils/code"
	"wcp/utils/config"
)

// outputSink is how a successful plain-text retrieval gets delivered.
// It never applies to a file-shaped retrieval (one carrying a
// wcp-name header) — those are always saved to disk, same as always.
type outputSink func([]byte)

// stdoutSink is `wcp <code>`'s own behavior — print to stdout.
func stdoutSink(data []byte) {
	emitText(data)
}

// clipboardSink is wcpv's behavior for a code that resolves to text:
// copy it instead of printing it. Falls back to printing (with a
// warning) if the clipboard can't be reached, so the content is never
// just lost.
func clipboardSink(data []byte) {
	if err := clipboard.Copy(string(data)); err != nil {
		printWarn(fmt.Sprintf("could not reach the clipboard; install %s — printing instead", clipboard.Hint()))
		emitText(data)
		return
	}
	printInfo("copied to clipboard")
}

// base64Body matches check_encrypted's charset check: once the
// "Salted__" marker (base64 "U2FsdGVkX1") is seen, the rest of the body
// must be base64 for it to be treated as our ciphertext rather than an
// unrelated upload that happens to start the same way.
var base64Body = regexp.MustCompile(`^[A-Za-z0-9+/=\s]*$`)

// shapeRE matches a plausible server-assigned id: 5-12 alphanumerics.
var shapeRE = regexp.MustCompile(`^[0-9a-zA-Z]{5,12}$`)

// Public functions

// implicitRetrieve tries candidate as a code; ok is false when it isn't
// shaped like one, meaning the caller should fall through to uploading
// it as text instead. An encrypted code that doesn't resolve is still
// "handled" (ok=true) — it must not silently become an upload.
func implicitRetrieve(f *Flags, candidate string, sink outputSink) (ok bool, exitCode int) {
	actual := candidate
	encKey := ""
	if i := strings.Index(candidate, "-"); i >= 0 {
		actual = candidate[:i]
		encKey = candidate[i+1:]
	}

	b, idFull, decodeOK := code.DecodeCode(actual)
	if !decodeOK {
		return false, 0
	}

	rest := actual[1:]
	shaped := false
	if len(candidate) <= 7 {
		shaped = shapeRE.MatchString(rest)
	} else if strings.ContainsAny(candidate, ".-") {
		shaped = shapeRE.MatchString(rest)
	}
	escapedExt := strings.Contains(actual, ".")

	if !shaped && !escapedExt {
		return false, 0
	}

	fetchHost := f.Host
	if fetchHost == "" {
		fetchHost = backend.FetchHost(b)
	}
	url := strings.TrimSuffix(fetchHost, "/") + "/" + idFull

	ctx, cancel := context.WithTimeout(context.Background(), backend.RetrieveMaxTime)
	defer cancel()

	body, contentType, err := backend.Retrieve(ctx, url)
	if err != nil {
		if encKey != "" {
			printWarn(fmt.Sprintf("no such code '%s' (%s)", actual, b))
			if b == code.Litterbox {
				printWarn("litterbox codes expire — the default window is 1h")
			}
			return true, 1
		}
		// Plain retrieval misses fall through to uploading the text.
		return false, 0
	}

	return true, handleRetrievedBody(f, body, encKey, f.OutFile, contentType, url, sink)
}

// explicitRetrieve handles `wcp . <code|url>` and `<code|url> -o <file>`
// — it never falls back to uploading, and errors loudly on a miss.
func explicitRetrieve(f *Flags, target, retrievalKey, outFile string, sink outputSink) int {
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return retrieveURLTarget(f, target, retrievalKey, outFile, sink)
	}

	actual := target
	encKey := retrievalKey
	if encKey == "" {
		if i := strings.Index(target, "-"); i >= 0 {
			actual = target[:i]
			encKey = target[i+1:]
		}
	}

	b, idFull, ok := code.DecodeCode(actual)
	if !ok {
		printError(fmt.Sprintf("'%s' is not a valid code (unknown prefix '%c')", target, actual[0]))
		return 1
	}

	fetchHost := f.Host
	if fetchHost == "" {
		fetchHost = backend.FetchHost(b)
	}
	url := strings.TrimSuffix(fetchHost, "/") + "/" + idFull

	ctx, cancel := context.WithTimeout(context.Background(), backend.RetrieveMaxTime)
	defer cancel()
	body, contentType, err := backend.Retrieve(ctx, url)
	if err != nil {
		printError(fmt.Sprintf("failed to fetch %s", url))
		return 1
	}

	return handleRetrievedBody(f, body, encKey, outFile, contentType, url, sink)
}

// Helpers

func retrieveURLTarget(f *Flags, target, retrievalKey, outFile string, sink outputSink) int {
	ctx, cancel := context.WithTimeout(context.Background(), backend.RetrieveMaxTime)
	defer cancel()

	body, contentType, err := backend.Retrieve(ctx, target)
	if err != nil {
		printError(fmt.Sprintf("failed to fetch %s", target))
		return 1
	}
	return handleRetrievedBody(f, body, retrievalKey, outFile, contentType, target, sink)
}

// handleRetrievedBody decides how to surface a retrieved body: decrypt
// with an explicit key if one came with the code, else fall back to the
// stored key when the body looks like our ciphertext, else treat it as
// plain content — matching retrieve_url/decrypt_and_emit's combined
// behavior in the bash client. sink only affects the plain-text leaf —
// an encrypted, wcp-name-headed retrieval always saves to disk
// regardless, and so does a plain retrieval whose Content-Type isn't
// text/* (matching retrieve_url's own ctype branch, since a plain
// upload carries no wcp-name header to recover a filename from).
func handleRetrievedBody(f *Flags, body []byte, explicitKey, outFile, contentType, url string, sink outputSink) int {
	if explicitKey != "" {
		return decryptAndEmit(f, explicitKey, body, outFile, sink)
	}
	if !isEncryptedBody(body) {
		return emitPlain(f, body, outFile, contentType, url, sink)
	}

	storedKey := ""
	if !f.NoStoredKey {
		var err error
		storedKey, err = config.ReadStoredKey()
		if err != nil {
			printError(err.Error())
			return 1
		}
	}
	if storedKey == "" {
		printError("this upload is encrypted, but no key is set on this machine")
		printError("run --set-key with the key from the machine that sent it")
		return 1
	}
	return decryptAndEmit(f, storedKey, body, outFile, sink)
}

// isEncryptedBody reports whether body looks like ciphertext we
// produced — base64 whose decoded form starts "Salted__" — recognising a
// stored-key upload, whose code carries no key.
func isEncryptedBody(body []byte) bool {
	if len(body) < 10 || string(body[:10]) != "U2FsdGVkX1" {
		return false
	}
	return base64Body.MatchString(strings.TrimSpace(string(body)))
}

// decryptAndEmit decrypts body with key, then saves it under its
// wcp-name header or hands it to sink, matching decrypt_and_emit. -o
// and -d both force a save even without a wcp-name header (there's
// nothing to sink if the user explicitly asked for a file); either
// way a save prints nothing but the resulting path.
func decryptAndEmit(f *Flags, key string, body []byte, overrideOut string, sink outputSink) int {
	plaintext, err := cipher.Decrypt(string(body), key)
	if err != nil {
		printError("could not decrypt — the key may be wrong or the upload corrupted")
		return 1
	}

	firstLine, rest := splitFirstLine(plaintext)
	hasName := strings.HasPrefix(firstLine, "wcp-name:")

	if overrideOut == "" && f.DownloadDir == "" && !hasName {
		sink(plaintext)
		return 0
	}

	data, name := plaintext, "download"
	if hasName {
		data, name = rest, strings.TrimPrefix(firstLine, "wcp-name:")
	}
	savedPath, err := saveUnderDownloadDir(f, name, data)
	if err != nil {
		printError(err.Error())
		return 1
	}
	printInfo(savedPath)
	return 0
}

// emitPlain delivers a plain (unencrypted) retrieval, matching
// retrieve_url's own branch exactly — a plain upload carries no
// wcp-name header (that's only added when encrypting), so there's no
// filename to recover from the body itself beyond the URL's basename.
// -o/-d force a save even for text/* content, same reasoning as
// decryptAndEmit above. Callers have already ruled out the
// encrypted-body case. A save prints nothing but the resulting path.
func emitPlain(f *Flags, body []byte, overrideOut, contentType, url string, sink outputSink) int {
	isText := strings.HasPrefix(contentType, "text/")
	if overrideOut == "" && f.DownloadDir == "" && isText {
		sink(body)
		return 0
	}

	fname := path.Base(url)
	if fname == "" || fname == "/" || fname == "." {
		fname = "downloaded_file"
	}
	savedPath, err := saveUnderDownloadDir(f, fname, body)
	if err != nil {
		printError(err.Error())
		return 1
	}
	printInfo(savedPath)
	return 0
}

// emitText prints retrieved text, adding a trailing newline only when
// stdout is a terminal and the content doesn't already end with one.
func emitText(data []byte) {
	os.Stdout.Write(data)
	if isTerminal(os.Stdout) && len(data) > 0 && data[len(data)-1] != '\n' {
		fmt.Println()
	}
}

func splitFirstLine(data []byte) (first string, rest []byte) {
	i := indexByte(data, '\n')
	if i < 0 {
		return string(data), nil
	}
	return string(data[:i]), data[i+1:]
}

func indexByte(data []byte, b byte) int {
	for i, c := range data {
		if c == b {
			return i
		}
	}
	return -1
}

// uniqueSaveName appends _1, _2, ... before the extension until the name
// is free, matching unique_save_name.
func uniqueSaveName(base string) string {
	if !fileExists(base) {
		return base
	}
	name, ext := base, ""
	if i := strings.LastIndex(base, "."); i > 0 {
		name, ext = base[:i], base[i+1:]
	}
	for i := 1; ; i++ {
		var candidate string
		if ext != "" {
			candidate = fmt.Sprintf("%s_%d.%s", name, i, ext)
		} else {
			candidate = fmt.Sprintf("%s_%d", name, i)
		}
		if !fileExists(candidate) {
			return candidate
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
