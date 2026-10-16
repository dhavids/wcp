// Package rtlog is a minimal JSONL command-history logger, trimmed
// from owrap's own utils/log/rtlog.py down to what wcp actually needs:
// append-only, size-rotated, one line per event, enough to answer
// `wcp get history N` without a database. Unlike rtlog.py, writes
// aren't file-locked (no portable equivalent across Linux/Windows
// without another dependency) — acceptable since wcp's own writers are
// short-lived CLI processes plus at most one hotkey daemon, not a
// busy multi-process server.
package rtlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wcp/utils/config"
)

// maxBytes is the size at which the log rotates to a single ".1"
// generation — simpler than rtlog.py's multi-generation scheme, since
// wcp's own log is small (short entries, local use only).
const maxBytes = 2 << 20 // 2 MiB

// Event is one JSONL line in the history log. RC is a pointer so a
// real 0 (success) round-trips distinctly from "not set" (a
// cmd.start event, which has no result yet).
type Event struct {
	TS    string  `json:"ts"`
	PID   int     `json:"pid"`
	Ev    string  `json:"ev"` // "cmd.start" or "cmd.end"
	Entry string  `json:"entry,omitempty"`
	Cmd   string  `json:"cmd,omitempty"`
	Argv  string  `json:"argv,omitempty"`
	RC    *int    `json:"rc,omitempty"`
	DurS  float64 `json:"dur_s,omitempty"`
	Note  string  `json:"note,omitempty"`
}

// Entry is one resolved history row — a cmd.end event with its
// matching cmd.start's argv attached, ready to print.
type Entry struct {
	TS    string
	Entry string
	Cmd   string
	Argv  string
	RC    int
	DurS  float64
	Note  string
}

// lastNote holds the most recent message passed to Note, consumed by
// the next End/Track call so ordinary printInfo/printError output
// doubles as the logged result without every call site needing to say
// so explicitly.
var lastNote string

// Public functions

// Note records msg as context for the next End/Track call in this
// process (printInfo/printError call this so their output becomes the
// logged note automatically).
func Note(msg string) {
	lastNote = msg
}

// Start logs that cmd began, with argv for later history display.
func Start(entry, cmd string, argv []string) {
	write(Event{
		TS: now(), PID: os.Getpid(), Ev: "cmd.start",
		Entry: entry, Cmd: cmd, Argv: strings.Join(argv, " "),
	})
}

// End logs that cmd finished with exit code rc, attaching whatever
// Note calls were made since Start and clearing them for next time.
func End(entry, cmd string, rc int, dur time.Duration) {
	rcCopy := rc
	write(Event{
		TS: now(), PID: os.Getpid(), Ev: "cmd.end",
		Entry: entry, Cmd: cmd, RC: &rcCopy, DurS: dur.Seconds(),
		Note: consumeNote(),
	})
}

// Track runs fn between a Start/End pair, timing it, and returns fn's
// exit code unchanged — the normal way to log one command.
func Track(entry, cmd string, argv []string, fn func() int) int {
	Start(entry, cmd, argv)
	t0 := time.Now()
	rc := fn()
	End(entry, cmd, rc, time.Since(t0))
	return rc
}

// History returns the last n completed commands (0 or negative means
// "all of them"), newest last, plus the total number ever recorded.
func History(n int) ([]Entry, int, error) {
	data, err := os.ReadFile(config.HistoryLogPath())
	if os.IsNotExist(err) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("rtlog: reading history: %w", err)
	}

	// Pair each cmd.end with the OLDEST still-unpaired cmd.start sharing
	// its pid, in file order — a plain "last start wins" map would
	// mismatch a long-lived process (the hotkey daemon) that logs many
	// sequential start/end pairs under the same pid.
	pending := map[int][]Event{}
	type paired struct {
		start, end Event
	}
	var all []paired
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		var e Event
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		switch e.Ev {
		case "cmd.start":
			pending[e.PID] = append(pending[e.PID], e)
		case "cmd.end":
			var start Event
			if q := pending[e.PID]; len(q) > 0 {
				start = q[0]
				pending[e.PID] = q[1:]
			}
			all = append(all, paired{start, e})
		}
	}

	total := len(all)
	if n <= 0 || n > total {
		n = total
	}
	shown := all[total-n:]

	out := make([]Entry, 0, len(shown))
	for _, p := range shown {
		rc := 0
		if p.end.RC != nil {
			rc = *p.end.RC
		}
		out = append(out, Entry{
			TS: p.end.TS, Entry: p.end.Entry, Cmd: p.end.Cmd,
			Argv: p.start.Argv, RC: rc, DurS: p.end.DurS, Note: p.end.Note,
		})
	}
	return out, total, nil
}

// Helpers

func consumeNote() string {
	n := lastNote
	lastNote = ""
	return n
}

func now() string {
	return time.Now().Format("2006-01-02T15:04:05.000")
}

// write appends one JSONL event, rotating the log first if it's grown
// past maxBytes. Failures are swallowed — a broken logger must never
// break the command it's trying to log.
func write(e Event) {
	path := config.HistoryLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	rotateIfNeeded(path)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()

	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	f.Write(append(line, '\n'))
}

func rotateIfNeeded(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxBytes {
		return
	}
	os.Rename(path, path+".1")
}
