// Package config manages wcp's on-disk state: the stored encryption key
// and the accumulate buffer, at the same XDG-based paths the bash/
// PowerShell clients already use, so an existing install's state survives
// the switch to the Go client untouched.
package config

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// MinStoredKeyLen is the shortest key WriteStoredKey will accept.
	MinStoredKeyLen = 20
	// DefaultKeyLen is the key length GenKey uses when none is given.
	DefaultKeyLen = 12
	// MaxUsernameLen is the longest username WriteStoredUser will accept.
	MaxUsernameLen = 32

	keyCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

// Public functions

// KeyPath returns the stored-key file path, honoring $XDG_CONFIG_HOME.
func KeyPath() string {
	return filepath.Join(xdgConfigHome(), "wcp", "key")
}

// BufferPath returns the accumulate-buffer file path, honoring
// $XDG_CACHE_HOME.
func BufferPath() string {
	return filepath.Join(xdgCacheHome(), "wcp", "buffer")
}

// UserPath returns the stored-username file path, honoring
// $XDG_CONFIG_HOME.
func UserPath() string {
	return filepath.Join(xdgConfigHome(), "wcp", "user")
}

// HotkeyComboPath returns the stored-combo file path for the given
// hotkey action ("copy" or "paste"), honoring $XDG_CONFIG_HOME.
func HotkeyComboPath(action string) string {
	return filepath.Join(xdgConfigHome(), "wcp", "hotkey-combo-"+action)
}

// HotkeyEnabledPath returns the hotkey-enabled marker file path for
// the given action, honoring $XDG_CONFIG_HOME. The file's existence,
// not its content, is what matters.
func HotkeyEnabledPath(action string) string {
	return filepath.Join(xdgConfigHome(), "wcp", "hotkey-enabled-"+action)
}

// HistoryLogPath returns the command-history log file path, honoring
// $XDG_CACHE_HOME — operational data, same category as the buffer.
func HistoryLogPath() string {
	return filepath.Join(xdgCacheHome(), "wcp", "history.log")
}

// ReadStoredKey returns the stored key, or "" if none is set.
func ReadStoredKey() (string, error) {
	data, err := os.ReadFile(KeyPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("config: reading stored key: %w", err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}

// WriteStoredKey validates and writes the key 0600 — no more exposed
// than an SSH private key. A stored key must start with a literal "A"
// so a `crid` string (`<username>-<key>`) can always be split on the
// first "-A", unambiguous even if the username contains a dash.
func WriteStoredKey(key string) error {
	if len(key) < MinStoredKeyLen {
		return fmt.Errorf("config: a stored key needs at least %d characters", MinStoredKeyLen)
	}
	if !isAlnum(key) {
		return errors.New("config: a stored key may only contain letters and digits")
	}
	if key[0] != 'A' {
		return errors.New("config: a stored key must start with 'A'")
	}

	path := KeyPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		return fmt.Errorf("config: writing stored key: %w", err)
	}
	return nil
}

// HasStoredKey reports whether a stored key file exists, without reading
// its contents.
func HasStoredKey() (bool, error) {
	_, err := os.Stat(KeyPath())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("config: %w", err)
	}
	return true, nil
}

// DeleteStoredKey removes the stored key, if any.
func DeleteStoredKey() error {
	err := os.Remove(KeyPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ReadStoredUser returns the stored username, or "" if none is set.
func ReadStoredUser() (string, error) {
	data, err := os.ReadFile(UserPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("config: reading stored user: %w", err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}

// WriteStoredUser validates and writes the username 0600. The charset
// (lowercase alnum + underscore, no uppercase, no dash) is restricted so
// a `crid` string (`<username>-<key>`) can always be split by looking
// for the literal `-A` that starts the key, with no ambiguity from a
// dash inside the username.
func WriteStoredUser(username string) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}

	path := UserPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := os.WriteFile(path, []byte(username+"\n"), 0o600); err != nil {
		return fmt.Errorf("config: writing stored user: %w", err)
	}
	return nil
}

// HasStoredUser reports whether a stored username file exists, without
// reading its contents.
func HasStoredUser() (bool, error) {
	_, err := os.Stat(UserPath())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("config: %w", err)
	}
	return true, nil
}

// DeleteStoredUser removes the stored username, if any.
func DeleteStoredUser() error {
	err := os.Remove(UserPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ValidateUsername reports whether username is a legal wcp username:
// 1 to MaxUsernameLen lowercase letters, digits, or underscores. This
// charset can never contain `-A`, keeping `crid` parsing unambiguous.
func ValidateUsername(username string) error {
	if username == "" {
		return errors.New("config: a username can't be empty")
	}
	if len(username) > MaxUsernameLen {
		return fmt.Errorf("config: a username is at most %d characters", MaxUsernameLen)
	}
	for _, r := range username {
		isLower := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		if !isLower && !isDigit && r != '_' {
			return errors.New("config: a username may only contain lowercase letters, digits, and underscores")
		}
	}
	return nil
}

// ReadHotkeyCombo returns the stored combo string for action, or "" if
// none is set (callers substitute their own default in that case).
// Storage here is a plain string — combo syntax is the hotkey
// package's job, not config's.
func ReadHotkeyCombo(action string) (string, error) {
	data, err := os.ReadFile(HotkeyComboPath(action))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("config: reading stored hotkey combo: %w", err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}

// WriteHotkeyCombo stores combo as-is for action; callers validate it
// first.
func WriteHotkeyCombo(action, combo string) error {
	path := HotkeyComboPath(action)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := os.WriteFile(path, []byte(combo+"\n"), 0o600); err != nil {
		return fmt.Errorf("config: writing stored hotkey combo: %w", err)
	}
	return nil
}

// DeleteHotkeyCombo removes action's stored combo, if any, resetting
// future reads back to the default.
func DeleteHotkeyCombo(action string) error {
	err := os.Remove(HotkeyComboPath(action))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// IsHotkeyEnabled reports whether action's hotkey is currently turned
// on, per the last `wcp set hotkey <action> on|off` call.
func IsHotkeyEnabled(action string) (bool, error) {
	_, err := os.Stat(HotkeyEnabledPath(action))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("config: %w", err)
	}
	return true, nil
}

// SetHotkeyEnabled records action's hotkey on/off state.
func SetHotkeyEnabled(action string, on bool) error {
	path := HotkeyEnabledPath(action)
	if !on {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		return fmt.Errorf("config: writing hotkey-enabled marker: %w", err)
	}
	return nil
}

// GenKey returns a random key of length n drawn from [A-Za-z0-9], using
// rejection sampling so every character stays equally likely.
func GenKey(n int) (string, error) {
	const maxUnbiased = 256 - 256%len(keyCharset)

	key := make([]byte, 0, n)
	buf := make([]byte, 1)
	for len(key) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("config: generating key: %w", err)
		}
		if int(buf[0]) >= maxUnbiased {
			continue
		}
		key = append(key, keyCharset[int(buf[0])%len(keyCharset)])
	}
	return string(key), nil
}

// AppendBuffer appends an entry, separated by two blank lines from
// anything already there.
func AppendBuffer(text string) error {
	path := BufferPath()
	info, statErr := os.Stat(path)
	hasContent := statErr == nil && info.Size() > 0

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("config: opening buffer: %w", err)
	}
	defer f.Close()

	if hasContent {
		_, err = fmt.Fprintf(f, "\n\n%s\n", text)
	} else {
		_, err = fmt.Fprintf(f, "%s\n", text)
	}
	if err != nil {
		return fmt.Errorf("config: appending to buffer: %w", err)
	}
	return nil
}

// ReadBuffer returns the buffer's raw contents, or "" if empty/missing.
func ReadBuffer() (string, error) {
	data, err := os.ReadFile(BufferPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("config: reading buffer: %w", err)
	}
	return string(data), nil
}

// ClearBuffer removes the buffer file.
func ClearBuffer() error {
	err := os.Remove(BufferPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// CountEntries counts buffer entries separated by two or more blank
// lines, matching the bash implementation's awk counter byte for byte
// (including its quirk of only resetting the blank-run count once it
// actually closes out an entry).
func CountEntries(content string) int {
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return 0
	}

	n := 1
	blanks := 0
	for _, line := range strings.Split(content, "\n") {
		if line == "" {
			blanks++
			continue
		}
		if blanks >= 2 {
			n++
			blanks = 0
		}
	}
	if blanks >= 2 {
		n++
	}
	return n
}

// Helpers

func isAlnum(s string) bool {
	for _, r := range s {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		if !isLetter && !isDigit {
			return false
		}
	}
	return true
}

func xdgConfigHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".config")
}

func xdgCacheHome() string {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".cache")
}
