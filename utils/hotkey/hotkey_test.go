package hotkey

import "testing"

func TestParseComboBasic(t *testing.T) {
	c, err := ParseCombo("alt+w")
	if err != nil {
		t.Fatalf("ParseCombo: %v", err)
	}
	if !c.Alt || c.Ctrl || c.Shift || c.Super || c.Key != "w" {
		t.Errorf("ParseCombo(alt+w) = %+v", c)
	}
}

func TestParseComboMultipleModifiersAndAliases(t *testing.T) {
	c, err := ParseCombo("Control+Shift+WIN+c")
	if err != nil {
		t.Fatalf("ParseCombo: %v", err)
	}
	if !c.Ctrl || !c.Shift || !c.Super || c.Alt || c.Key != "c" {
		t.Errorf("ParseCombo(Control+Shift+WIN+c) = %+v", c)
	}
}

func TestParseComboRejectsBadInput(t *testing.T) {
	cases := []string{"", "alt", "alt+", "alt+ws", "ctrl+banana+w", "+w"}
	for _, s := range cases {
		if _, err := ParseCombo(s); err == nil {
			t.Errorf("ParseCombo(%q) = nil error, want one", s)
		}
	}
}

func TestComboStringRoundTrips(t *testing.T) {
	cases := []string{"w", "alt+w", "ctrl+alt+shift+super+c"}
	for _, s := range cases {
		c, err := ParseCombo(s)
		if err != nil {
			t.Fatalf("ParseCombo(%q): %v", s, err)
		}
		c2, err := ParseCombo(c.String())
		if err != nil {
			t.Fatalf("ParseCombo(%q) (round trip): %v", c.String(), err)
		}
		if c != c2 {
			t.Errorf("round trip mismatch: %+v != %+v", c, c2)
		}
	}
}
