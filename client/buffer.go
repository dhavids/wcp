package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"wcp/utils/code"
	"wcp/utils/config"
)

// runBufferAction handles --clear, -a and -s. stdin is read lazily via
// readStdin, since -a only consumes it when there are no positional
// args and the input isn't a terminal.
func runBufferAction(f *Flags, readStdin func() ([]byte, error)) int {
	warnPermanentPlaintext(f)

	if f.ClearBuffer {
		return doClearBuffer()
	}
	if f.Accumulate {
		return doAccumulate(f, readStdin)
	}
	return doSend(f)
}

// Helpers

func warnPermanentPlaintext(f *Flags) {
	if !f.Encrypt && f.Backend == code.Catbox && (f.Plain || f.NoStoredKey) {
		printWarn("uploading to catbox UNENCRYPTED - catbox never expires")
	}
}

func doClearBuffer() int {
	content, err := config.ReadBuffer()
	if err != nil {
		printError(err.Error())
		return 1
	}
	if content == "" {
		printInfo("buffer cleared (0 entries, 0 B)")
		return 0
	}
	entries := config.CountEntries(content)
	size := humanSize(len(content))
	if err := config.ClearBuffer(); err != nil {
		printError(err.Error())
		return 1
	}
	printInfo(fmt.Sprintf("buffer cleared (%s, %s)", pluralEntries(entries), size))
	return 0
}

func doAccumulate(f *Flags, readStdin func() ([]byte, error)) int {
	if len(f.Args) > 0 {
		text := strings.Join(f.Args, " ")
		if err := config.AppendBuffer(text); err != nil {
			printError(err.Error())
			return 1
		}
		return 0
	}

	if isTerminal(os.Stdin) {
		return showBuffer(f.Full)
	}

	raw, err := readStdin()
	if err != nil {
		printError(err.Error())
		return 1
	}
	if len(raw) == 0 {
		return showBuffer(f.Full)
	}
	if bytes.IndexByte(raw, 0) >= 0 {
		printError("only text can be accumulated")
		return 1
	}
	if err := config.AppendBuffer(string(raw)); err != nil {
		printError(err.Error())
		return 1
	}
	return 0
}

func doSend(f *Flags) int {
	content, err := config.ReadBuffer()
	if err != nil {
		printError(err.Error())
		return 1
	}
	if content == "" {
		printError("buffer is empty - nothing to send")
		return 1
	}

	shortCode, exitCode := uploadText(f, content, "")
	if exitCode != 0 {
		return exitCode
	}
	if err := config.ClearBuffer(); err != nil {
		printError(err.Error())
		return 1
	}
	printInfo(shortCode)
	return 0
}

// showBuffer prints head+tail (or everything with --full, off a
// terminal, or when short), followed by an entry/size summary.
func showBuffer(full bool) int {
	content, err := config.ReadBuffer()
	if err != nil {
		printError(err.Error())
		return 1
	}
	if content == "" {
		printWarn("buffer is empty - nothing to show")
		return 0
	}

	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if full || !isTerminal(os.Stdout) || len(lines) <= 10 {
		fmt.Print(content)
	} else {
		fmt.Println(strings.Join(lines[:5], "\n"))
		fmt.Printf("... %d lines hidden, --full shows everything ...\n", len(lines)-10)
		fmt.Println(strings.Join(lines[len(lines)-5:], "\n"))
	}

	entries := config.CountEntries(content)
	printInfo(fmt.Sprintf("%s, %s", pluralEntries(entries), humanSize(len(content))))
	return 0
}

func pluralEntries(n int) string {
	if n == 1 {
		return "1 entry"
	}
	return fmt.Sprintf("%d entries", n)
}

func humanSize(bytesLen int) string {
	switch {
	case bytesLen >= 1<<20:
		return fmt.Sprintf("%d MB", bytesLen/(1<<20))
	case bytesLen >= 1<<10:
		return fmt.Sprintf("%d KB", (bytesLen+512)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytesLen)
	}
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func readAllStdin() ([]byte, error) {
	return io.ReadAll(os.Stdin)
}
