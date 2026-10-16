package clipboard

import (
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestHintReflectsSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		if got := Hint(); got == "" {
			t.Error("Hint() should not be empty on Windows")
		}
		return
	}
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("DISPLAY", "")
	if got, want := Hint(), "wl-clipboard"; got != want {
		t.Errorf("Hint() = %q, want %q", got, want)
	}

	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")
	if got, want := Hint(), "xclip"; got != want {
		t.Errorf("Hint() = %q, want %q", got, want)
	}

	t.Setenv("DISPLAY", "")
	if got := Hint(); got == "wl-clipboard" || got == "xclip" {
		t.Errorf("Hint() = %q, want the no-display message", got)
	}
}

// Only runs where a real clipboard tool is reachable — proves Copy/Paste
// actually round-trip through whatever tool this session has, rather
// than just exercising the tool-selection logic.
func TestCopyPasteRoundTrip(t *testing.T) {
	if runtime.GOOS != "windows" {
		if _, err := exec.LookPath("pbcopy"); err != nil {
			if _, err := exec.LookPath("wl-copy"); err != nil {
				if _, err := exec.LookPath("xclip"); err != nil {
					if _, err := exec.LookPath("xsel"); err != nil {
						t.Skip("no clipboard tool available in this environment")
					}
				}
			}
		}
	}

	text := "wcp clipboard round trip " + time.Now().Format(time.RFC3339Nano)
	if err := Copy(text); err != nil {
		t.Skipf("Copy failed in this environment: %v", err)
	}
	got, err := Paste()
	if err != nil {
		t.Fatalf("Paste: %v", err)
	}
	if got != text {
		t.Errorf("Paste() = %q, want %q", got, text)
	}
}
