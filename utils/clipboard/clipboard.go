// Package clipboard reads and writes the OS clipboard by shelling out to
// the same tools the bash/PowerShell clients already document —
// pbcopy/pbpaste, wl-copy/wl-paste, xclip, xsel, and clip.exe/
// Get-Clipboard on Windows — rather than a cgo clipboard binding.
package clipboard

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"runtime"
)

// ErrNoTool means no usable clipboard tool was found for this session —
// e.g. an SSH session with no display, or none of the optional tools
// installed.
var ErrNoTool = errors.New("clipboard: no usable clipboard tool found")

// Public functions

// Copy writes text to the OS clipboard.
func Copy(text string) error {
	if runtime.GOOS == "windows" {
		return runPipe("clip", nil, text)
	}
	return copyPOSIX(text)
}

// Paste returns the OS clipboard's current contents.
func Paste() (string, error) {
	if runtime.GOOS == "windows" {
		return runCapture("powershell", []string{"-NoProfile", "-Command", "Get-Clipboard -Raw"})
	}
	return pastePOSIX()
}

// Hint names what to install for the current session, for an error
// message when no tool was found.
func Hint() string {
	if runtime.GOOS == "windows" {
		return "clip.exe (ships with Windows)"
	}
	if hasDisplay("WAYLAND_DISPLAY") {
		return "wl-clipboard"
	}
	if hasDisplay("DISPLAY") {
		return "xclip"
	}
	return "a clipboard tool, but this session has no display"
}

// Helpers

func copyPOSIX(text string) error {
	if hasTool("pbcopy") {
		return runPipe("pbcopy", nil, text)
	}
	if hasDisplay("WAYLAND_DISPLAY") && hasTool("wl-copy") {
		return runPipe("wl-copy", nil, text)
	}
	if hasDisplay("DISPLAY") && hasTool("xclip") {
		return runPipe("xclip", []string{"-selection", "clipboard"}, text)
	}
	if hasDisplay("DISPLAY") && hasTool("xsel") {
		return runPipe("xsel", []string{"--clipboard", "--input"}, text)
	}
	return ErrNoTool
}

func pastePOSIX() (string, error) {
	if hasTool("pbpaste") {
		return runCapture("pbpaste", nil)
	}
	if hasDisplay("WAYLAND_DISPLAY") && hasTool("wl-paste") {
		return runCapture("wl-paste", []string{"--no-newline"})
	}
	if hasDisplay("DISPLAY") && hasTool("xclip") {
		return runCapture("xclip", []string{"-selection", "clipboard", "-o"})
	}
	if hasDisplay("DISPLAY") && hasTool("xsel") {
		return runCapture("xsel", []string{"--clipboard", "--output"})
	}
	return "", ErrNoTool
}

func hasTool(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func hasDisplay(envVar string) bool {
	return os.Getenv(envVar) != ""
}

func runPipe(name string, args []string, text string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = bytes.NewBufferString(text)
	return cmd.Run()
}

func runCapture(name string, args []string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
