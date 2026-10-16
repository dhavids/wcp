//go:build windows

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"

	"wcp/utils/hotkey"
)

// Win32 globalhotkey plumbing: no cgo, just syscall.NewLazyDLL against
// user32.dll (the same pattern x/sys/windows/svc's own beep example
// uses for MessageBeep) plus a hand-rolled MSG struct, since user32's
// GUI-adjacent APIs aren't part of x/sys/windows itself.

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procRegisterHotKey   = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procKeybdEvent       = user32.NewProc("keybd_event")
)

const (
	modAlt     = 0x0001
	modControl = 0x0002
	modShift   = 0x0004
	modWin     = 0x0008
	wmHotkey   = 0x0312

	vkControl     = 0x11
	vkC           = 0x43
	keyEventKeyUp = 0x0002

	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	runKeyName = "wcp-hotkey"
)

// point and msg mirror the Win32 POINT/MSG structs exactly (natural
// field alignment on amd64/arm64 matches the C layout, including the
// implicit padding before the pointer-sized WParam/LParam fields).
type point struct {
	x, y int32
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

func platformSupported() (bool, string) {
	return true, ""
}

func installAutostart(execPath string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(runKeyName, fmt.Sprintf(`"%s" --hotkey-daemon`, execPath))
}

func removeAutostart() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(runKeyName); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

// runHotkeyDaemon registers every combo in combos as a thread-level
// hotkey (hwnd NULL, so WM_HOTKEY lands on this thread's own message
// queue — no window needed), each under its own id, and blocks on
// GetMessage, calling onTrigger with the matching action on every hit.
func runHotkeyDaemon(combos map[string]hotkey.Combo, onTrigger func(action string)) error {
	// RegisterHotKey/GetMessage must run on the same OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ids := map[int]string{}
	id := 1
	for action, combo := range combos {
		vk, err := vkCode(combo.Key)
		if err != nil {
			return err
		}
		mods := winModifiers(combo)

		ret, _, callErr := procRegisterHotKey.Call(0, uintptr(id), uintptr(mods), uintptr(vk))
		if ret == 0 {
			return fmt.Errorf("hotkey: RegisterHotKey(%s) failed: %v", action, callErr)
		}
		ids[id] = action
		id++
	}
	defer func() {
		for hid := range ids {
			procUnregisterHotKey.Call(0, uintptr(hid))
		}
	}()

	var m msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			return nil
		}
		if m.message == wmHotkey {
			if action, ok := ids[int(m.wParam)]; ok {
				onTrigger(action)
			}
		}
	}
}

// simulateCopy synthesizes a Ctrl+C keypress via the legacy
// keybd_event API — used by the "copy" hotkey to copy the user's
// current selection before uploading it, rather than only uploading
// whatever was already on the clipboard. keybd_event is deprecated in
// favor of SendInput, but its trivial signature (no struct layout to
// get exactly right, unlike SendInput's INPUT union) makes it the
// safer choice here without a real Windows machine to verify against.
// Always succeeds (no realistic failure mode worth checking for this).
func simulateCopy() bool {
	procKeybdEvent.Call(uintptr(vkControl), 0, 0, 0)
	procKeybdEvent.Call(uintptr(vkC), 0, 0, 0)
	procKeybdEvent.Call(uintptr(vkC), 0, uintptr(keyEventKeyUp), 0)
	procKeybdEvent.Call(uintptr(vkControl), 0, uintptr(keyEventKeyUp), 0)
	return true
}

// Helpers

// vkCode maps combo.Key (always one letter or digit, per
// hotkey.ParseCombo) to its Win32 virtual-key code — for letters and
// digits this is just the ASCII uppercase/digit code.
func vkCode(key string) (uint32, error) {
	if len(key) != 1 {
		return 0, fmt.Errorf("hotkey: bad key %q", key)
	}
	c := key[0]
	switch {
	case c >= '0' && c <= '9':
		return uint32(c), nil
	case c >= 'a' && c <= 'z':
		return uint32(c - 'a' + 'A'), nil
	default:
		return 0, fmt.Errorf("hotkey: unsupported key %q", key)
	}
}

func winModifiers(c hotkey.Combo) uint32 {
	var m uint32
	if c.Alt {
		m |= modAlt
	}
	if c.Ctrl {
		m |= modControl
	}
	if c.Shift {
		m |= modShift
	}
	if c.Super {
		m |= modWin
	}
	return m
}
