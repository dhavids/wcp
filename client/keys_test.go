package main

import (
	"path/filepath"
	"testing"

	"wcp/utils/config"
)

func sandboxIdentity(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
}

func TestIsVerbCommand(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"set", "key", "abc"}, true},
		{[]string{"get", "crid"}, true},
		{[]string{"clear", "user"}, true},
		{[]string{"set", "hotkey", "copy"}, true},
		{[]string{"get", "history"}, true},
		{[]string{"get", "history", "10"}, true},
		{[]string{"set", "history"}, false},
		{[]string{"notes.txt"}, false},
		{[]string{"set"}, false},
		{[]string{}, false},
	}
	for _, c := range cases {
		if got := isVerbCommand(c.args); got != c.want {
			t.Errorf("isVerbCommand(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestParseCridRoundTrip(t *testing.T) {
	username, key, err := parseCrid(formatCrid("dhavid", "Akeyvalueabc123456789"))
	if err != nil {
		t.Fatalf("parseCrid: %v", err)
	}
	if username != "dhavid" || key != "Akeyvalueabc123456789" {
		t.Errorf("parseCrid() = (%q, %q)", username, key)
	}
}

func TestParseCridRejectsMissingKeyPrefix(t *testing.T) {
	if _, _, err := parseCrid("dhavid-notakey"); err == nil {
		t.Error("parseCrid should reject a value whose key half doesn't start with A")
	}
}

func TestSetGetClearKeyVerbs(t *testing.T) {
	sandboxIdentity(t)

	if code := runVerb([]string{"set", "key"}); code != 0 {
		t.Fatalf("set key (generate) exit = %d", code)
	}
	key, err := config.ReadStoredKey()
	if err != nil || key == "" || key[0] != 'A' {
		t.Fatalf("generated key = %q, err %v; want non-empty starting with A", key, err)
	}

	if code := runVerb([]string{"clear", "key"}); code != 0 {
		t.Fatalf("clear key exit = %d", code)
	}
	if got, _ := config.ReadStoredKey(); got != "" {
		t.Errorf("after clear, stored key = %q, want empty", got)
	}
}

func TestSetGetClearCridVerbs(t *testing.T) {
	sandboxIdentity(t)

	crid := formatCrid("dhavid_s", "A"+mustGenKey(t, 20))
	if code := runVerb([]string{"set", "crid", crid}); code != 0 {
		t.Fatalf("set crid exit = %d", code)
	}

	user, _ := config.ReadStoredUser()
	if user != "dhavid_s" {
		t.Errorf("stored user = %q, want dhavid_s", user)
	}

	if code := runVerb([]string{"clear", "crid"}); code != 0 {
		t.Fatalf("clear crid exit = %d", code)
	}
	if gotUser, _ := config.ReadStoredUser(); gotUser != "" {
		t.Errorf("after clear crid, user = %q, want empty", gotUser)
	}
	if gotKey, _ := config.ReadStoredKey(); gotKey != "" {
		t.Errorf("after clear crid, key = %q, want empty", gotKey)
	}
}

func mustGenKey(t *testing.T, n int) string {
	t.Helper()
	key, err := config.GenKey(n)
	if err != nil {
		t.Fatalf("GenKey: %v", err)
	}
	return key
}
