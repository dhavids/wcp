package rtlog

import (
	"path/filepath"
	"testing"
)

func sandbox(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
}

func TestHistoryEmpty(t *testing.T) {
	sandbox(t)
	entries, total, err := History(5)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if entries != nil || total != 0 {
		t.Errorf("History() = %v, %d; want nil, 0", entries, total)
	}
}

func TestTrackRecordsSuccessAndFailure(t *testing.T) {
	sandbox(t)

	rc := Track("cli", "upload", []string{"notes.txt"}, func() int {
		Note("b0ojyr4")
		return 0
	})
	if rc != 0 {
		t.Fatalf("Track() = %d, want 0", rc)
	}

	rc = Track("hotkey", "hotkey-paste", nil, func() int {
		Note("wcpp needs the self-hosted backend")
		return 1
	})
	if rc != 1 {
		t.Fatalf("Track() = %d, want 1", rc)
	}

	entries, total, err := History(5)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if total != 2 {
		t.Fatalf("History() total = %d, want 2", total)
	}
	if len(entries) != 2 {
		t.Fatalf("History() len = %d, want 2", len(entries))
	}

	first, second := entries[0], entries[1]
	if first.Cmd != "upload" || first.Entry != "cli" || first.RC != 0 || first.Argv != "notes.txt" {
		t.Errorf("entries[0] = %+v", first)
	}
	if first.Note != "b0ojyr4" {
		t.Errorf("entries[0].Note = %q, want b0ojyr4", first.Note)
	}
	if second.Cmd != "hotkey-paste" || second.Entry != "hotkey" || second.RC != 1 {
		t.Errorf("entries[1] = %+v", second)
	}
	if second.Note != "wcpp needs the self-hosted backend" {
		t.Errorf("entries[1].Note = %q", second.Note)
	}
}

func TestHistoryRespectsCount(t *testing.T) {
	sandbox(t)
	for i := 0; i < 7; i++ {
		Track("cli", "noop", nil, func() int { return 0 })
	}

	entries, total, err := History(3)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if total != 7 {
		t.Errorf("total = %d, want 7", total)
	}
	if len(entries) != 3 {
		t.Errorf("len(entries) = %d, want 3", len(entries))
	}
}
