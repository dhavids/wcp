//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgb/xtest"

	"wcp/utils/hotkey"
)

// keysymControlL is X11's Control_L keysym — not in the Latin-1 range
// keycodeForKey handles, since Ctrl is a modifier key, not a
// printable character.
const keysymControlL = 0xffe3

// X11-only for now (Debian/Ubuntu target) — Wayland compositors have no
// XGrabKey equivalent without a desktop-portal global-shortcuts API,
// which is skipped in v1 (see scratch.md). The autostart mechanism is a
// systemd --user unit; daemon start/stop itself is handled by the
// shared pidfile code in hotkey.go, so the unit only needs to be
// enabled for future logins, not started here.

const unitName = "wcp-hotkey.service"

func platformSupported() (bool, string) {
	if os.Getenv("XDG_SESSION_TYPE") == "wayland" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return false, "this is a Wayland session — the hotkey daemon needs X11, no portal fallback yet"
	}
	if os.Getenv("DISPLAY") == "" {
		return false, "no X11 DISPLAY is set — the hotkey daemon needs an X11 session"
	}
	return true, ""
}

func systemdUserDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}

func unitPath() (string, error) {
	dir, err := systemdUserDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, unitName), nil
}

func installAutostart(execPath string) error {
	dir, err := systemdUserDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	path, err := unitPath()
	if err != nil {
		return err
	}
	unit := fmt.Sprintf(`[Unit]
Description=wcp hotkey daemon

[Service]
ExecStart=%s --hotkey-daemon
Restart=on-failure

[Install]
WantedBy=default.target
`, execPath)
	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		return err
	}

	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("systemctl --user daemon-reload: %w", err)
	}
	if err := exec.Command("systemctl", "--user", "enable", unitName).Run(); err != nil {
		return fmt.Errorf("systemctl --user enable %s: %w", unitName, err)
	}
	return nil
}

func removeAutostart() error {
	// Ignore "not enabled"/"not found" — removing something already
	// gone is a no-op, not a failure of set hotkey off.
	exec.Command("systemctl", "--user", "disable", unitName).Run()

	path, err := unitPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return exec.Command("systemctl", "--user", "daemon-reload").Run()
}

// grabbedKey tracks one action's XGrabKey registration, so an incoming
// KeyPressEvent can be matched back to the action that should fire.
type grabbedKey struct {
	keycode xproto.Keycode
	mods    uint16
	action  string
}

// runHotkeyDaemon grabs every combo in combos globally via XGrabKey
// and blocks on the X11 event stream, calling onTrigger with the
// matching action on every KeyPress that matches one of them.
func runHotkeyDaemon(combos map[string]hotkey.Combo, onTrigger func(action string)) error {
	X, err := xgb.NewConn()
	if err != nil {
		return fmt.Errorf("hotkey: connecting to X11: %w", err)
	}
	defer X.Close()

	setup := xproto.Setup(X)
	root := setup.DefaultScreen(X).Root

	var grabs []grabbedKey
	for action, combo := range combos {
		keycode, err := keycodeForKey(X, setup, combo.Key)
		if err != nil {
			return err
		}
		mods := x11Modifiers(combo)

		if err := xproto.GrabKeyChecked(X, true, root, mods, keycode, xproto.GrabModeAsync, xproto.GrabModeAsync).Check(); err != nil {
			return fmt.Errorf("hotkey: XGrabKey(%s) failed: %w", action, err)
		}
		grabs = append(grabs, grabbedKey{keycode, mods, action})
	}
	defer func() {
		for _, g := range grabs {
			xproto.UngrabKey(X, g.keycode, root, g.mods)
		}
	}()

	for {
		ev, err := X.WaitForEvent()
		if err != nil {
			continue
		}
		kp, ok := ev.(xproto.KeyPressEvent)
		if !ok {
			continue
		}
		for _, g := range grabs {
			if kp.Detail == g.keycode && kp.State&g.mods == g.mods {
				onTrigger(g.action)
				break
			}
		}
	}
}

// Helpers

// keycodeForKey maps combo.Key (one lowercase letter or digit) to its
// X11 keycode. For these characters, the Latin-1 keysym value equals
// the ASCII byte itself, so this is just keycodeForKeysym on that byte.
func keycodeForKey(X *xgb.Conn, setup *xproto.SetupInfo, key string) (xproto.Keycode, error) {
	return keycodeForKeysym(X, setup, xproto.Keysym(key[0]))
}

// keycodeForKeysym scans the keyboard mapping for the keycode whose
// keysym list contains target — the general form keycodeForKey and
// simulateCopy's Control_L lookup both reduce to.
func keycodeForKeysym(X *xgb.Conn, setup *xproto.SetupInfo, target xproto.Keysym) (xproto.Keycode, error) {
	count := byte(int(setup.MaxKeycode) - int(setup.MinKeycode) + 1)
	reply, err := xproto.GetKeyboardMapping(X, setup.MinKeycode, count).Reply()
	if err != nil {
		return 0, fmt.Errorf("hotkey: GetKeyboardMapping: %w", err)
	}

	perKeycode := int(reply.KeysymsPerKeycode)
	if perKeycode == 0 {
		return 0, fmt.Errorf("hotkey: keyboard mapping reported 0 keysyms per keycode")
	}
	for i, ks := range reply.Keysyms {
		if ks == target {
			return xproto.Keycode(int(setup.MinKeycode) + i/perKeycode), nil
		}
	}
	return 0, fmt.Errorf("hotkey: no keycode found for keysym %#x", target)
}

// simulateCopy synthesizes a Ctrl+C keypress via the XTEST extension,
// targeting whatever window currently has focus — used by the "copy"
// hotkey to copy the user's current selection before uploading it,
// rather than only uploading whatever was already on the clipboard.
// Uses its own short-lived connection, separate from the daemon's main
// grab connection. Returns false (caller falls back to the existing
// clipboard contents) if XTEST isn't available.
func simulateCopy() bool {
	X, err := xgb.NewConn()
	if err != nil {
		return false
	}
	defer X.Close()

	if err := xtest.Init(X); err != nil {
		return false
	}

	setup := xproto.Setup(X)
	ctrlCode, err := keycodeForKeysym(X, setup, keysymControlL)
	if err != nil {
		return false
	}
	cCode, err := keycodeForKeysym(X, setup, xproto.Keysym('c'))
	if err != nil {
		return false
	}

	xtest.FakeInput(X, xproto.KeyPress, byte(ctrlCode), 0, 0, 0, 0, 0)
	xtest.FakeInput(X, xproto.KeyPress, byte(cCode), 0, 0, 0, 0, 0)
	xtest.FakeInput(X, xproto.KeyRelease, byte(cCode), 0, 0, 0, 0, 0)
	xtest.FakeInput(X, xproto.KeyRelease, byte(ctrlCode), 0, 0, 0, 0, 0)
	return true
}

func x11Modifiers(c hotkey.Combo) uint16 {
	var m uint16
	if c.Shift {
		m |= xproto.ModMaskShift
	}
	if c.Ctrl {
		m |= xproto.ModMaskControl
	}
	if c.Alt {
		m |= xproto.ModMask1 // Mod1 is the conventional Alt modifier
	}
	if c.Super {
		m |= xproto.ModMask4 // Mod4 is the conventional Super/Win modifier
	}
	return m
}
