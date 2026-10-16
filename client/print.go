package main

import (
	"fmt"
	"os"

	"wcp/utils/rtlog"
)

// printInfo/printError also feed rtlog.Note, so whichever one a
// command calls last before returning doubles as its logged history
// result — no extra bookkeeping needed at most call sites.

// lastErrorMsg is the most recent printError message, read by the
// hotkey daemon to put a failure on the clipboard when there's no
// terminal to print to — a separate capture from rtlog's own Note,
// since Track's End() consumes that one internally before the trigger
// closure gets a chance to read it.
var lastErrorMsg string

func printInfo(msg string) {
	fmt.Fprintln(os.Stdout, msg)
	rtlog.Note(msg)
}

func printWarn(msg string) {
	fmt.Fprintln(os.Stderr, "wcp:", msg)
}

func printError(msg string) {
	fmt.Fprintln(os.Stderr, "wcp:", msg)
	rtlog.Note(msg)
	lastErrorMsg = msg
}
