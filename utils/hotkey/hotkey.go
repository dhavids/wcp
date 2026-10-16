// Package hotkey parses and canonicalizes global-hotkey combo strings
// (e.g. "alt+w"), shared by the `wcp set/get/clear hotkey` CLI and each
// platform's daemon implementation.
package hotkey

import (
	"fmt"
	"strings"
)

// DefaultCombo is the combo used for the "copy" (wcpc-trigger) action
// when none has been configured. Ctrl+Alt+<letter>, not a bare
// Alt+<letter> — Alt+W collides with VS Code's Find-widget "toggle
// whole word" shortcut, and bare Alt+<letter> combos generally double
// as menu mnemonics in classic Windows apps.
const DefaultCombo = "ctrl+alt+c"

// DefaultPasteCombo is the combo used for the "paste" (wcpp-trigger)
// action when none has been configured — distinct from DefaultCombo so
// both actions can be turned on at once without colliding by default.
const DefaultPasteCombo = "ctrl+alt+v"

// Combo is a parsed global-hotkey combination: zero or more modifiers
// plus exactly one key.
type Combo struct {
	Ctrl  bool
	Alt   bool
	Shift bool
	Super bool
	Key   string // single lowercase letter or digit, e.g. "w"
}

// Public functions

// ParseCombo parses a "+"-separated combo string such as "alt+w" or
// "ctrl+shift+c". Modifier names are case-insensitive; "control", "win",
// "cmd", and "meta" are accepted as aliases for ctrl/super. The key
// itself must be exactly one letter or digit.
func ParseCombo(s string) (Combo, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), "+")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return Combo{}, fmt.Errorf("hotkey: %q isn't a valid combo (e.g. alt+w)", s)
	}

	var c Combo
	key := parts[len(parts)-1]
	for _, p := range parts[:len(parts)-1] {
		switch p {
		case "ctrl", "control":
			c.Ctrl = true
		case "alt":
			c.Alt = true
		case "shift":
			c.Shift = true
		case "super", "win", "cmd", "meta":
			c.Super = true
		default:
			return Combo{}, fmt.Errorf("hotkey: unknown modifier %q in %q", p, s)
		}
	}
	if len(key) != 1 || !isAlnumByte(key[0]) {
		return Combo{}, fmt.Errorf("hotkey: the key in %q must be exactly one letter or digit", s)
	}
	c.Key = key
	return c, nil
}

// String returns the canonical form of c, e.g. "ctrl+alt+w" — modifiers
// always in ctrl/alt/shift/super order, then the key.
// ParseCombo(c.String()) round-trips.
func (c Combo) String() string {
	var parts []string
	if c.Ctrl {
		parts = append(parts, "ctrl")
	}
	if c.Alt {
		parts = append(parts, "alt")
	}
	if c.Shift {
		parts = append(parts, "shift")
	}
	if c.Super {
		parts = append(parts, "super")
	}
	parts = append(parts, c.Key)
	return strings.Join(parts, "+")
}

// Helpers

func isAlnumByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}
