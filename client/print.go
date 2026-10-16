package main

import (
	"fmt"
	"os"

	"wcp/utils/rtlog"
)

// printInfo/printError also feed rtlog.Note, so whichever one a
// command calls last before returning doubles as its logged history
// result — no extra bookkeeping needed at most call sites.

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
}
