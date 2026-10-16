package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wcp/utils/clipboard"
	"wcp/utils/config"
	"wcp/utils/hotkey"
	"wcp/utils/rtlog"
)

// Two independent hotkey actions: "copy" triggers wcpc (clipboard →
// upload), "paste" triggers wcpp (download → clipboard). Both are
// registered by the same daemon process when enabled.
const (
	actionCopy  = "copy"
	actionPaste = "paste"
)

// isHotkeyDaemon is true for the whole lifetime of a process started
// with --hotkey-daemon, so entryPoint() can tell a hotkey-triggered
// wcpc/wcpp call apart from a normal CLI invocation.
var isHotkeyDaemon bool

func entryPoint() string {
	if isHotkeyDaemon {
		return "hotkey"
	}
	return "cli"
}

// errNotSupported is a small shared helper so each platform's "this
// isn't implemented here" error reads the same way everywhere.
func errNotSupported(reason string) error {
	return errors.New(reason)
}

// runHotkeyVerb handles `wcp get/set/clear hotkey`.
func runHotkeyVerb(verb string, rest []string) int {
	switch verb {
	case "set":
		return runSetHotkey(rest)
	case "get":
		if len(rest) > 0 {
			printError("get hotkey takes no arguments")
			return 1
		}
		return doGetHotkey()
	default:
		if len(rest) > 0 {
			printError("clear hotkey takes no arguments")
			return 1
		}
		return doClearHotkey()
	}
}

// runHotkeyDaemonMode is the daemon's own entry point, reached via the
// hidden --hotkey-daemon flag that installAutostart/startDaemonBackground
// launch the binary with. Redirects this process's own stdout/stderr to
// a log file first, since it normally runs with nothing attached.
func runHotkeyDaemonMode() int {
	isHotkeyDaemon = true
	if f, err := os.OpenFile(hotkeyLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		os.Stdout = f
		os.Stderr = f
	}

	combos, err := enabledCombos()
	if err != nil {
		printError("hotkey daemon: " + err.Error())
		return 1
	}
	if len(combos) == 0 {
		printError("hotkey daemon: no action is enabled, exiting")
		return 1
	}

	printInfo("hotkey daemon started: " + describeCombos(combos))
	err = runHotkeyDaemon(combos, func(action string) {
		cmd := "hotkey-" + action
		rc := rtlog.Track(entryPoint(), cmd, nil, func() int {
			switch action {
			case actionCopy:
				simulateCopyAndWait()
				return runWcpc(nil)
			default:
				return runWcpp()
			}
		})
		if rc != 0 {
			printError(fmt.Sprintf("hotkey trigger (%s): failed, rc=%d", action, rc))
		} else {
			printInfo(fmt.Sprintf("hotkey trigger (%s): succeeded", action))
		}
	})
	if err != nil {
		printError("hotkey daemon: " + err.Error())
		return 1
	}
	return 0
}

// Helpers

// simulateCopyAndWait synthesizes a Ctrl+C (via the platform's
// simulateCopy) so the "copy" hotkey uploads the user's current
// selection, not just whatever was already on the clipboard, then
// polls briefly for the clipboard to actually change. If simulateCopy
// isn't supported on this platform, or nothing changes within the
// deadline (nothing was selected, or it didn't reach the focused
// window), it simply returns — the caller then uploads whatever's on
// the clipboard regardless, same as before this feature existed.
func simulateCopyAndWait() {
	before, _ := clipboard.Paste()
	if !simulateCopy() {
		return
	}

	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		after, err := clipboard.Paste()
		if err == nil && after != before {
			return
		}
	}
}

func runSetHotkey(rest []string) int {
	if len(rest) < 2 {
		printError("set hotkey needs an action and on/off, e.g. wcp set hotkey copy on")
		return 1
	}
	action := rest[0]
	if action != actionCopy && action != actionPaste {
		printError(fmt.Sprintf("set hotkey's action must be %q or %q, got %q", actionCopy, actionPaste, action))
		return 1
	}

	switch rest[1] {
	case "on":
		if len(rest) > 3 {
			printError("set hotkey <action> on takes at most one combo argument")
			return 1
		}
		combo := ""
		if len(rest) == 3 {
			combo = rest[2]
		}
		return doSetHotkeyOn(action, combo)
	case "off":
		if len(rest) > 2 {
			printError("set hotkey <action> off takes no arguments")
			return 1
		}
		return doSetHotkeyOff(action)
	default:
		printError("set hotkey needs on or off, e.g. wcp set hotkey copy on ctrl+shift+c")
		return 1
	}
}

func doSetHotkeyOn(action, comboArg string) int {
	if ok, reason := platformSupported(); !ok {
		printError("hotkey daemon not supported here: " + reason)
		return 1
	}

	comboStr := comboArg
	if comboStr == "" {
		if stored, err := config.ReadHotkeyCombo(action); err == nil && stored != "" {
			comboStr = stored
		} else {
			comboStr = defaultComboFor(action)
		}
	}
	combo, err := hotkey.ParseCombo(comboStr)
	if err != nil {
		printError(err.Error())
		return 1
	}

	if other := otherAction(action); other != "" {
		if onOther, _ := config.IsHotkeyEnabled(other); onOther {
			if otherCombo, _ := config.ReadHotkeyCombo(other); otherCombo == combo.String() {
				printError(fmt.Sprintf("%s is already using %q for %s — pick a different combo", other, combo.String(), other))
				return 1
			}
		}
	}

	if err := config.WriteHotkeyCombo(action, combo.String()); err != nil {
		printError(err.Error())
		return 1
	}
	if err := config.SetHotkeyEnabled(action, true); err != nil {
		printError(err.Error())
		return 1
	}

	if err := restartDaemonForCurrentState(); err != nil {
		printError("hotkey: " + err.Error())
		return 1
	}

	printInfo(fmt.Sprintf("hotkey %s on (%s)", action, combo.String()))
	if action == actionPaste {
		warnIfNoHost()
	}
	return 0
}

// warnIfNoHost is a non-blocking heads-up for `set hotkey paste on`:
// unlike copy (wcpc, works against any backend), paste (wcpp) only
// works against a self-hosted backend with a user+key configured —
// enabling the hotkey without one is still allowed (it's fine to set
// up ahead of time), but it would otherwise fail silently until then.
func warnIfNoHost() {
	user, _ := config.ReadStoredUser()
	key, _ := config.ReadStoredKey()
	if user == "" || key == "" {
		printWarn("paste needs a configured user+key against a self-hosted backend (wcp set user/key) — " +
			"the hotkey is on, but wcpp will fail until then")
	}
}

func doSetHotkeyOff(action string) int {
	if err := config.SetHotkeyEnabled(action, false); err != nil {
		printError(err.Error())
		return 1
	}
	if err := restartDaemonForCurrentState(); err != nil {
		printError("hotkey: " + err.Error())
		return 1
	}
	printInfo("hotkey " + action + " off")
	return 0
}

func doGetHotkey() int {
	for _, action := range []string{actionCopy, actionPaste} {
		on, err := config.IsHotkeyEnabled(action)
		if err != nil {
			printError(err.Error())
			return 1
		}
		comboStr, err := config.ReadHotkeyCombo(action)
		if err != nil {
			printError(err.Error())
			return 1
		}
		if comboStr == "" {
			comboStr = defaultComboFor(action)
		}
		state := "off"
		if on {
			state = "on"
		}
		fmt.Printf("%s: %s (%s)\n", action, state, comboStr)
	}
	return 0
}

func doClearHotkey() int {
	for _, action := range []string{actionCopy, actionPaste} {
		if err := config.DeleteHotkeyCombo(action); err != nil {
			printError(err.Error())
			return 1
		}
	}
	printInfo(fmt.Sprintf("hotkey combos reset to default (copy=%s, paste=%s)", hotkey.DefaultCombo, hotkey.DefaultPasteCombo))
	return 0
}

func defaultComboFor(action string) string {
	if action == actionPaste {
		return hotkey.DefaultPasteCombo
	}
	return hotkey.DefaultCombo
}

func otherAction(action string) string {
	if action == actionCopy {
		return actionPaste
	}
	return actionCopy
}

// enabledCombos resolves every currently-enabled action to its parsed
// combo — what the daemon should register on startup.
func enabledCombos() (map[string]hotkey.Combo, error) {
	result := map[string]hotkey.Combo{}
	for _, action := range []string{actionCopy, actionPaste} {
		on, err := config.IsHotkeyEnabled(action)
		if err != nil {
			return nil, err
		}
		if !on {
			continue
		}
		comboStr, err := config.ReadHotkeyCombo(action)
		if err != nil {
			return nil, err
		}
		if comboStr == "" {
			comboStr = defaultComboFor(action)
		}
		combo, err := hotkey.ParseCombo(comboStr)
		if err != nil {
			return nil, err
		}
		result[action] = combo
	}
	return result, nil
}

func describeCombos(combos map[string]hotkey.Combo) string {
	var parts []string
	for action, combo := range combos {
		parts = append(parts, action+"="+combo.String())
	}
	return strings.Join(parts, ", ")
}

// restartDaemonForCurrentState stops whatever daemon is running and,
// if any action is still enabled, starts a fresh one so it picks up
// the latest combo set; installs/removes autostart to match.
func restartDaemonForCurrentState() error {
	stopRunningDaemon()

	combos, err := enabledCombos()
	if err != nil {
		return err
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	if len(combos) == 0 {
		return removeAutostart()
	}
	if err := installAutostart(exePath); err != nil {
		return fmt.Errorf("installing autostart: %w", err)
	}
	if err := startDaemonBackground(exePath); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}
	return nil
}

// hotkeyStateDir is where the daemon's own pidfile/log live — the same
// directory the stored combo/key/user files already use.
func hotkeyStateDir() string {
	return filepath.Dir(config.HotkeyComboPath(actionCopy))
}

func hotkeyPidPath() string {
	return filepath.Join(hotkeyStateDir(), "hotkey.pid")
}

func hotkeyLogPath() string {
	return filepath.Join(hotkeyStateDir(), "hotkey.log")
}

// startDaemonBackground launches exePath --hotkey-daemon detached from
// this process, recording its pid so stopRunningDaemon can find it
// later. No OS-specific code needed — os/exec and os.Process.Kill
// already do the right thing on both Windows and Linux.
func startDaemonBackground(exePath string) error {
	cmd := exec.Command(exePath, "--hotkey-daemon")
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := os.MkdirAll(hotkeyStateDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(hotkeyPidPath(), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600)
}

// stopRunningDaemon best-effort kills whatever pid was last recorded;
// errors are swallowed since "nothing was running" is a normal case,
// not a failure of set hotkey off.
func stopRunningDaemon() {
	data, err := os.ReadFile(hotkeyPidPath())
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err == nil {
		if proc, err := os.FindProcess(pid); err == nil {
			proc.Kill()
		}
	}
	os.Remove(hotkeyPidPath())
}
