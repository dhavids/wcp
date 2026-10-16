package main

import "testing"

func TestParseArgsBackendShorthand(t *testing.T) {
	f, err := ParseArgs([]string{"-b", "l"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if f.Backend != "litterbox" {
		t.Errorf("Backend = %q, want litterbox", f.Backend)
	}
}

func TestParseArgsInlineEquals(t *testing.T) {
	f, err := ParseArgs([]string{"--backend=c", "--host=https://example.test", "-t=24"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if f.Backend != "catbox" {
		t.Errorf("Backend = %q, want catbox", f.Backend)
	}
	if f.Host != "https://example.test" {
		t.Errorf("Host = %q", f.Host)
	}
	if f.LBHours != 24 {
		t.Errorf("LBHours = %d, want 24", f.LBHours)
	}
}

func TestParseArgsPositionalAndDoubleDash(t *testing.T) {
	f, err := ParseArgs([]string{"-n", "--", "-not-a-flag", "more text"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if !f.NoCopy {
		t.Error("NoCopy should be set")
	}
	if len(f.Args) != 2 || f.Args[0] != "-not-a-flag" || f.Args[1] != "more text" {
		t.Errorf("Args = %#v", f.Args)
	}
}

func TestParseArgsOutputFlag(t *testing.T) {
	f, err := ParseArgs([]string{"b0ojyr4", "-o", "notes.txt"})
	if err != nil {
		t.Fatalf("ParseArgs: %v", err)
	}
	if f.OutFile != "notes.txt" {
		t.Errorf("OutFile = %q", f.OutFile)
	}
	if len(f.Args) != 1 || f.Args[0] != "b0ojyr4" {
		t.Errorf("Args = %#v", f.Args)
	}
}
