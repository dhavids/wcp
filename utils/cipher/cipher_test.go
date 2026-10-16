package cipher

import (
	"bytes"
	"os/exec"
	"testing"
)

const testPassword = "test-password-correct-horse"

func hasOpenSSL() bool {
	_, err := exec.LookPath("openssl")
	return err == nil
}

func TestRoundTrip(t *testing.T) {
	plaintext := []byte("wcp-name:notes.txt\nhello from the Go cipher package\n")

	encoded, err := Encrypt(plaintext, testPassword)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	decoded, err := Decrypt(encoded, testPassword)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(decoded, plaintext) {
		t.Fatalf("round trip mismatch: got %q, want %q", decoded, plaintext)
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	encoded, err := Encrypt([]byte("some data"), testPassword)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := Decrypt(encoded, "wrong-password"); err == nil {
		t.Fatal("Decrypt with the wrong password should fail")
	}
}

// Proves the Go implementation can decrypt ciphertext produced by the
// exact openssl invocation the bash/PowerShell clients use.
func TestDecryptOpenSSLCiphertext(t *testing.T) {
	if !hasOpenSSL() {
		t.Skip("openssl not available")
	}

	plaintext := "wcp-name:from-openssl.txt\nproduced by real openssl\n"
	cmd := exec.Command("openssl", "enc", "-aes-256-cbc", "-pbkdf2", "-iter", "10000",
		"-salt", "-pass", "pass:"+testPassword, "-base64", "-A")
	cmd.Stdin = bytes.NewBufferString(plaintext)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("openssl enc: %v", err)
	}

	decoded, err := Decrypt(string(out), testPassword)
	if err != nil {
		t.Fatalf("Decrypt(openssl output): %v", err)
	}
	if string(decoded) != plaintext {
		t.Fatalf("got %q, want %q", decoded, plaintext)
	}
}

// Proves real openssl can decrypt ciphertext the Go implementation
// produces — compatibility has to hold in both directions during the
// migration, not just Go reading old data.
func TestOpenSSLDecryptsGoCiphertext(t *testing.T) {
	if !hasOpenSSL() {
		t.Skip("openssl not available")
	}

	plaintext := []byte("wcp-name:from-go.txt\nproduced by the Go cipher package\n")
	encoded, err := Encrypt(plaintext, testPassword)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	cmd := exec.Command("openssl", "enc", "-d", "-aes-256-cbc", "-pbkdf2", "-iter", "10000",
		"-pass", "pass:"+testPassword, "-base64", "-A")
	cmd.Stdin = bytes.NewBufferString(encoded)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("openssl enc -d: %v", err)
	}
	if !bytes.Equal(out, plaintext) {
		t.Fatalf("got %q, want %q", out, plaintext)
	}
}
