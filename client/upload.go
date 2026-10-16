package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"wcp/utils/backend"
	"wcp/utils/cipher"
	"wcp/utils/clipboard"
	"wcp/utils/code"
	"wcp/utils/config"
)

// uploadConfig is the per-run encryption/backend configuration, resolved
// once from flags, the stored key and env vars (configure_upload in the
// bash client).
type uploadConfig struct {
	storedKey string
	encrypt   bool
	lbTime    string
}

// resolveUploadConfig validates flag combinations and decides whether
// this upload is encrypted, matching configure_upload's decision table
// exactly: -u (plain) wins, then -e, then a stored key, then litterbox
// being plain by default, then everything else encrypted by default.
func resolveUploadConfig(f *Flags) (uploadConfig, error) {
	if f.Permanent {
		return uploadConfig{}, fmt.Errorf("-p/--permanent needs the self-hosted wcp-host backend, not deployed yet")
	}
	if f.KeyLen < 12 || f.KeyLen > 64 {
		return uploadConfig{}, fmt.Errorf("--key-len must be a whole number from 12 to 64 (default 12)")
	}

	lbTime, err := litterboxTimeBucket(f.LBHours)
	if err != nil {
		return uploadConfig{}, err
	}

	storedKey := ""
	if !f.NoStoredKey {
		storedKey, err = config.ReadStoredKey()
		if err != nil {
			return uploadConfig{}, err
		}
	}

	encrypt := true
	switch {
	case f.Plain:
		encrypt = false
	case f.Encrypt:
		encrypt = true
	case storedKey != "":
		encrypt = true
	case f.Backend == code.Litterbox:
		encrypt = false
	}

	return uploadConfig{storedKey: storedKey, encrypt: encrypt, lbTime: lbTime}, nil
}

// uploadText uploads plain text (used by the text-argument path, stdin,
// -v/--paste, and -s/--send) and returns the string to print — the short
// code, or a CODE-KEY pair when no key is stored.
func uploadText(f *Flags, text string, _ string) (string, int) {
	return runUpload(f, strings.NewReader(text), "paste.txt", "")
}

// uploadFile uploads a real file from disk, applying the encrypted-size
// ceiling (MAX_ENC_SIZE, raised to MAX_FORCE_SIZE by -f) and prefixing
// the wcp-name header so retrieval restores the original filename.
func uploadFile(f *Flags, path string) int {
	info, err := os.Stat(path)
	if err != nil {
		printError(err.Error())
		return 1
	}

	cfg, err := resolveUploadConfig(f)
	if err != nil {
		printError(err.Error())
		return 1
	}

	size := info.Size()
	if cfg.encrypt && size > maxEncSize && !f.Force {
		printError(fmt.Sprintf(
			"%s is over the %s encrypted limit — pass -f to raise it to %s, or -p to upload as-is",
			humanSize(int(size)), humanSize(maxEncSize), humanSize(maxForceSize)))
		return 1
	}
	if cfg.encrypt && size > maxForceSize {
		printError(fmt.Sprintf("-f cannot encrypt it - %s is over the %s encrypted limit",
			humanSize(int(size)), humanSize(maxForceSize)))
		return 1
	}

	file, err := os.Open(path)
	if err != nil {
		printError(err.Error())
		return 1
	}
	defer file.Close()

	name := filepath.Base(path)
	emitted, exitCode := runUploadWithConfig(f, cfg, file, name, name)
	if exitCode != 0 {
		return exitCode
	}
	printInfo(emitted)
	return 0
}

// uploadStdin uploads raw stdin bytes (no wcp-name header, matching the
// bash stdin path).
func uploadStdin(f *Flags, data io.Reader) int {
	emitted, exitCode := runUpload(f, data, "stdin.txt", "")
	if exitCode != 0 {
		return exitCode
	}
	printInfo(emitted)
	return 0
}

// Helpers

func runUpload(f *Flags, data io.Reader, uploadFilename, wcpName string) (string, int) {
	cfg, err := resolveUploadConfig(f)
	if err != nil {
		printError(err.Error())
		return "", 1
	}
	return runUploadWithConfig(f, cfg, data, uploadFilename, wcpName)
}

func runUploadWithConfig(f *Flags, cfg uploadConfig, data io.Reader, uploadFilename, wcpName string) (string, int) {
	raw, err := io.ReadAll(data)
	if err != nil {
		printError(err.Error())
		return "", 1
	}

	key := cfg.storedKey
	emitKey := ""
	body := raw
	if cfg.encrypt {
		// Only an encrypted upload needs its filename smuggled inside
		// the plaintext (as a wcp-name header) — the server can't see
		// it otherwise. A plain upload keeps its real multipart
		// filename and Content-Type, so retrieval doesn't need this.
		plaintext := raw
		if wcpName != "" {
			plaintext = append([]byte("wcp-name:"+wcpName+"\n"), raw...)
		}
		if key == "" {
			key, err = config.GenKey(f.KeyLen)
			if err != nil {
				printError(err.Error())
				return "", 1
			}
			emitKey = key
		}
		encoded, err := cipher.Encrypt(plaintext, key)
		if err != nil {
			printError(err.Error())
			return "", 1
		}
		body = []byte(encoded)
	}

	ctx, cancel := context.WithTimeout(context.Background(), backend.UploadMaxTime)
	defer cancel()

	opts := backend.Options{Backend: f.Backend, Host: f.Host, LitterboxTTL: cfg.lbTime}
	idFull, err := backend.Upload(ctx, opts, uploadFilename, bytes.NewReader(body))
	if err != nil {
		printError(err.Error())
		return "", 1
	}

	return emitResult(f, idFull, emitKey), 0
}

// emitResult builds the printed line (short code, CODE-KEY, or a full
// URL under -l) and copies the part the user would paste to the
// clipboard, matching emit_result/maybe_copy.
func emitResult(f *Flags, idFull, key string) string {
	shortCode := code.EncodeID(idFull, f.Backend)

	var out string
	if f.Link {
		host := f.Host
		if host == "" {
			host = backend.FetchHost(f.Backend)
		}
		out = strings.TrimSuffix(host, "/") + "/" + idFull
		maybeCopy(f, out)
		if key != "" {
			return out + "\n" + key
		}
		return out
	}

	if key != "" {
		out = shortCode + "-" + key
	} else {
		out = shortCode
	}
	maybeCopy(f, out)
	return out
}

// maybeCopy copies by default on a terminal; -c forces it even when
// redirected, -n/WCP_NO_COPY disables it entirely.
func maybeCopy(f *Flags, text string) {
	if f.NoCopy {
		return
	}
	if !f.Copy && !isTerminal(os.Stdout) {
		return
	}
	if err := clipboard.Copy(text); err != nil && f.Copy {
		printWarn(fmt.Sprintf("-c could not reach the clipboard; install %s", clipboard.Hint()))
	}
}

func litterboxTimeBucket(hours int) (string, error) {
	switch {
	case hours < 1:
		return "", fmt.Errorf("-t takes a whole number of hours, 1 to 72")
	case hours <= 1:
		return "1h", nil
	case hours <= 12:
		return "12h", nil
	case hours <= 24:
		return "24h", nil
	case hours <= 72:
		return "72h", nil
	default:
		return "", fmt.Errorf("-t max is 72 hours (litterbox allows 1, 12, 24, 72)")
	}
}

const (
	maxEncSize   = 1048576
	maxForceSize = 104857600
)
