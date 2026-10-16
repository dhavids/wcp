package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sandbox(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
}

func TestKeyPathsHonorXDG(t *testing.T) {
	sandbox(t)
	if got, want := KeyPath(), filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "wcp", "key"); got != want {
		t.Errorf("KeyPath() = %q, want %q", got, want)
	}
	if got, want := BufferPath(), filepath.Join(os.Getenv("XDG_CACHE_HOME"), "wcp", "buffer"); got != want {
		t.Errorf("BufferPath() = %q, want %q", got, want)
	}
}

func TestReadStoredKeyMissingReturnsEmpty(t *testing.T) {
	sandbox(t)
	key, err := ReadStoredKey()
	if err != nil {
		t.Fatalf("ReadStoredKey: %v", err)
	}
	if key != "" {
		t.Errorf("ReadStoredKey() = %q, want empty", key)
	}
}

func TestWriteAndReadStoredKey(t *testing.T) {
	sandbox(t)
	const key = "AbcdefghijklmnopqrstuvwxyzABCDEF01"
	if err := WriteStoredKey(key); err != nil {
		t.Fatalf("WriteStoredKey: %v", err)
	}

	info, err := os.Stat(KeyPath())
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("key file mode = %o, want 600", perm)
	}

	got, err := ReadStoredKey()
	if err != nil {
		t.Fatalf("ReadStoredKey: %v", err)
	}
	if got != key {
		t.Errorf("ReadStoredKey() = %q, want %q", got, key)
	}

	if err := DeleteStoredKey(); err != nil {
		t.Fatalf("DeleteStoredKey: %v", err)
	}
	if got, _ := ReadStoredKey(); got != "" {
		t.Errorf("after delete, ReadStoredKey() = %q, want empty", got)
	}
}

func TestWriteStoredKeyRejectsShortOrInvalid(t *testing.T) {
	sandbox(t)
	if err := WriteStoredKey("tooshort"); err == nil {
		t.Error("WriteStoredKey should reject a key under MinStoredKeyLen")
	}
	if err := WriteStoredKey("thisiskeyislongenoughbut-has-a-dash"); err == nil {
		t.Error("WriteStoredKey should reject non-alphanumeric characters")
	}
}

func TestWriteAndReadStoredUser(t *testing.T) {
	sandbox(t)
	const user = "dhavid_s"
	if err := WriteStoredUser(user); err != nil {
		t.Fatalf("WriteStoredUser: %v", err)
	}

	got, err := ReadStoredUser()
	if err != nil {
		t.Fatalf("ReadStoredUser: %v", err)
	}
	if got != user {
		t.Errorf("ReadStoredUser() = %q, want %q", got, user)
	}

	if err := DeleteStoredUser(); err != nil {
		t.Fatalf("DeleteStoredUser: %v", err)
	}
	if got, _ := ReadStoredUser(); got != "" {
		t.Errorf("after delete, ReadStoredUser() = %q, want empty", got)
	}
}

func TestValidateUsernameRejectsBadCharsetOrEmpty(t *testing.T) {
	cases := []string{"", "Dhavid", "no-dash", "has space", "has.dot"}
	for _, c := range cases {
		if err := ValidateUsername(c); err == nil {
			t.Errorf("ValidateUsername(%q) = nil, want an error", c)
		}
	}
	if err := ValidateUsername(strings.Repeat("a", MaxUsernameLen+1)); err == nil {
		t.Error("ValidateUsername should reject a username over MaxUsernameLen")
	}
	if err := ValidateUsername("valid_user_42"); err != nil {
		t.Errorf("ValidateUsername(valid_user_42) = %v, want nil", err)
	}
}

func TestHotkeyComboAndEnabledState(t *testing.T) {
	sandbox(t)
	const action = "copy"

	if got, err := ReadHotkeyCombo(action); err != nil || got != "" {
		t.Fatalf("ReadHotkeyCombo() = %q, %v; want empty, nil", got, err)
	}
	if on, err := IsHotkeyEnabled(action); err != nil || on {
		t.Fatalf("IsHotkeyEnabled() = %v, %v; want false, nil", on, err)
	}

	if err := WriteHotkeyCombo(action, "ctrl+shift+c"); err != nil {
		t.Fatalf("WriteHotkeyCombo: %v", err)
	}
	if got, err := ReadHotkeyCombo(action); err != nil || got != "ctrl+shift+c" {
		t.Fatalf("ReadHotkeyCombo() = %q, %v; want ctrl+shift+c", got, err)
	}

	if err := SetHotkeyEnabled(action, true); err != nil {
		t.Fatalf("SetHotkeyEnabled(true): %v", err)
	}
	if on, err := IsHotkeyEnabled(action); err != nil || !on {
		t.Fatalf("IsHotkeyEnabled() = %v, %v; want true, nil", on, err)
	}

	if err := SetHotkeyEnabled(action, false); err != nil {
		t.Fatalf("SetHotkeyEnabled(false): %v", err)
	}
	if on, _ := IsHotkeyEnabled(action); on {
		t.Error("IsHotkeyEnabled() = true after disabling")
	}

	if err := DeleteHotkeyCombo(action); err != nil {
		t.Fatalf("DeleteHotkeyCombo: %v", err)
	}
	if got, _ := ReadHotkeyCombo(action); got != "" {
		t.Errorf("after delete, ReadHotkeyCombo() = %q, want empty", got)
	}
}

func TestHotkeyActionsAreIndependent(t *testing.T) {
	sandbox(t)

	if err := WriteHotkeyCombo("copy", "alt+w"); err != nil {
		t.Fatalf("WriteHotkeyCombo(copy): %v", err)
	}
	if err := WriteHotkeyCombo("paste", "alt+e"); err != nil {
		t.Fatalf("WriteHotkeyCombo(paste): %v", err)
	}
	if got, _ := ReadHotkeyCombo("copy"); got != "alt+w" {
		t.Errorf("ReadHotkeyCombo(copy) = %q, want alt+w", got)
	}
	if got, _ := ReadHotkeyCombo("paste"); got != "alt+e" {
		t.Errorf("ReadHotkeyCombo(paste) = %q, want alt+e", got)
	}
}

func TestGenKeyLengthAndCharset(t *testing.T) {
	key, err := GenKey(32)
	if err != nil {
		t.Fatalf("GenKey: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("GenKey length = %d, want 32", len(key))
	}
	if !isAlnum(key) {
		t.Errorf("GenKey() = %q, contains non-alphanumeric characters", key)
	}
}

func TestBufferAppendReadClear(t *testing.T) {
	sandbox(t)

	if err := AppendBuffer("first"); err != nil {
		t.Fatalf("AppendBuffer: %v", err)
	}
	if err := AppendBuffer("second"); err != nil {
		t.Fatalf("AppendBuffer: %v", err)
	}

	got, err := ReadBuffer()
	if err != nil {
		t.Fatalf("ReadBuffer: %v", err)
	}
	if want := "first\n\n\nsecond\n"; got != want {
		t.Errorf("ReadBuffer() = %q, want %q", got, want)
	}
	if n := CountEntries(got); n != 2 {
		t.Errorf("CountEntries() = %d, want 2", n)
	}

	if err := ClearBuffer(); err != nil {
		t.Fatalf("ClearBuffer: %v", err)
	}
	if got, _ := ReadBuffer(); got != "" {
		t.Errorf("after clear, ReadBuffer() = %q, want empty", got)
	}
}

func TestCountEntries(t *testing.T) {
	cases := []struct {
		content string
		want    int
	}{
		{"", 0},
		{"one entry\n", 1},
		{"one\n\n\ntwo\n", 2},
		{"one\n\n\ntwo\n\n\nthree\n", 3},
	}
	for _, c := range cases {
		if got := CountEntries(c.content); got != c.want {
			t.Errorf("CountEntries(%q) = %d, want %d", c.content, got, c.want)
		}
	}
}
