package main

import (
	"fmt"
	"strconv"

	"wcp/utils/rtlog"
)

// defaultHistoryCount matches owrap's own `get history` default.
const defaultHistoryCount = 5

// doGetHistory handles `wcp get history [n]`, printing the last n
// logged commands — entry point, status, duration, and whatever note
// printInfo/printError last left — in owrap's own history format.
func doGetHistory(rest []string) int {
	if len(rest) > 1 {
		printError("get history takes at most one count argument")
		return 1
	}
	n := defaultHistoryCount
	if len(rest) == 1 {
		v, err := strconv.Atoi(rest[0])
		if err != nil || v <= 0 {
			printError("get history's count must be a positive whole number")
			return 1
		}
		n = v
	}

	entries, total, err := rtlog.History(n)
	if err != nil {
		printError(err.Error())
		return 1
	}
	if total == 0 {
		fmt.Println("no history yet")
		return 0
	}

	fmt.Printf("history (last %d of %d):\n", len(entries), total)
	for _, e := range entries {
		status := "ok"
		if e.RC != 0 {
			status = fmt.Sprintf("rc=%d", e.RC)
		}
		ts := e.TS
		if len(ts) >= 19 {
			ts = ts[11:19]
		}
		fmt.Printf("  %-14s [%-6s]  done(%s)  %.2fs  @ %s\n", e.Cmd, e.Entry, status, e.DurS, ts)
		if e.Argv != "" {
			fmt.Printf("    argv: %s\n", e.Argv)
		}
		if e.Note != "" {
			fmt.Printf("    note: %s\n", e.Note)
		}
	}
	return 0
}
