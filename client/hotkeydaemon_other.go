//go:build !windows && !linux

package main

import "wcp/utils/hotkey"

// The hotkey daemon isn't implemented outside Windows and Debian/
// Ubuntu Linux yet (see docs/research/projects/wcp/scratch.md) — macOS
// and anything else gets a clear, immediate "not supported" instead of
// silently doing nothing.

func platformSupported() (bool, string) {
	return false, "the hotkey daemon only supports Windows and Debian/Ubuntu Linux so far"
}

func installAutostart(execPath string) error {
	_, reason := platformSupported()
	return errNotSupported(reason)
}

func removeAutostart() error {
	return nil
}

func runHotkeyDaemon(combos map[string]hotkey.Combo, onTrigger func(action string)) error {
	_, reason := platformSupported()
	return errNotSupported(reason)
}

func simulateCopy() bool {
	return false
}
