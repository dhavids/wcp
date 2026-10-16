// Command wcp uploads a file, some text, or stdin, and prints back a
// short code; given a code, it retrieves instead. The same binary also
// answers to wcpc/wcpp/wcpv, chosen by argv[0] — see dispatch.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wcp/utils/clipboard"
	"wcp/utils/rtlog"
)

func main() {
	os.Exit(dispatch(os.Args))
}

// Helpers

// dispatch picks a mode from argv[0]'s basename, the same way
// busybox/toybox multiplex one binary under several names.
func dispatch(args []string) int {
	mode := strings.TrimSuffix(filepath.Base(args[0]), ".exe")
	switch mode {
	case "wcpc":
		return rtlog.Track(entryPoint(), "wcpc", args, func() int { return runWcpc(args[1:]) })
	case "wcpp":
		return rtlog.Track(entryPoint(), "wcpp", args, func() int { return runWcpp() })
	case "wcpv":
		return rtlog.Track(entryPoint(), "wcpv", args, func() int { return runWcpv(args[1:]) })
	default:
		return runWcp(args[1:])
	}
}

// runWcp is the full CLI's entry point once argv[0] has been resolved to
// the default mode.
func runWcp(args []string) int {
	if isVerbCommand(args) {
		return runVerb(args)
	}

	f, err := ParseArgs(args)
	if err != nil {
		printError(err.Error())
		return 1
	}
	if f.Help {
		fmt.Print(Usage())
		return 0
	}
	if f.Update {
		return rtlog.Track(entryPoint(), "update", args, func() int { return doUpdate() })
	}
	if f.HotkeyDaemon {
		return runHotkeyDaemonMode()
	}
	if f.Accumulate || f.Send || f.ClearBuffer {
		return rtlog.Track(entryPoint(), bufferCmdName(f), args, func() int { return runBufferAction(f, readAllStdin) })
	}
	if f.Paste {
		return rtlog.Track(entryPoint(), "paste", args, func() int { return runPasteUpload(f) })
	}

	if len(f.Args) > 0 && f.Args[0] == "." {
		return rtlog.Track(entryPoint(), "retrieve", args, func() int { return runExplicitDot(f) })
	}
	if f.OutFile != "" && len(f.Args) == 1 {
		return rtlog.Track(entryPoint(), "retrieve", args, func() int {
			return explicitRetrieve(f, f.Args[0], "", f.OutFile, stdoutSink)
		})
	}
	if len(f.Args) == 1 && !fileExists(f.Args[0]) {
		var handled bool
		rc := rtlog.Track(entryPoint(), "retrieve", args, func() int {
			ok, code := implicitRetrieve(f, f.Args[0], stdoutSink)
			handled = ok
			return code
		})
		if handled {
			return rc
		}
	}

	return rtlog.Track(entryPoint(), "upload", args, func() int { return doUpload(f) })
}

// bufferCmdName picks the history label for whichever buffer flag f
// carries — accumulate, send, or clear are mutually exclusive here.
func bufferCmdName(f *Flags) string {
	switch {
	case f.Send:
		return "buffer-send"
	case f.ClearBuffer:
		return "buffer-clear"
	default:
		return "buffer-accumulate"
	}
}

// runPasteUpload handles -v/--paste: upload whatever is on the local
// clipboard right now, as if it had arrived on stdin.
func runPasteUpload(f *Flags) int {
	if len(f.Args) > 0 {
		printError("-v/--paste takes no arguments (it uploads the clipboard)")
		return 1
	}
	text, err := clipboard.Paste()
	if err != nil || text == "" {
		printError(fmt.Sprintf("-v found no clipboard tool; install %s", clipboard.Hint()))
		return 1
	}
	return uploadStdin(f, strings.NewReader(text))
}

// runExplicitDot handles `wcp . <code|url> [key]`, including a key
// carried in a URL fragment (#key).
func runExplicitDot(f *Flags) int {
	rest := f.Args[1:]
	if len(rest) == 0 {
		printError(". needs a code or URL, e.g. wcp . b0ojyr4")
		return 1
	}
	target, key := rest[0], ""
	if len(rest) > 1 {
		key = rest[1]
	}
	if i := strings.Index(target, "#"); i >= 0 {
		key = target[i+1:]
		target = target[:i]
	}
	return explicitRetrieve(f, target, key, f.OutFile, stdoutSink)
}

// doUpload decides, from the positional args alone, whether this is a
// file, stdin, or text upload.
func doUpload(f *Flags) int {
	switch {
	case len(f.Args) == 0:
		return uploadFromStdin(f)
	case len(f.Args) == 1 && fileExists(f.Args[0]):
		return uploadFile(f, f.Args[0])
	default:
		emitted, exitCode := uploadText(f, strings.Join(f.Args, " "), "")
		if exitCode == 0 {
			printInfo(emitted)
		}
		return exitCode
	}
}

func uploadFromStdin(f *Flags) int {
	if isTerminal(os.Stdin) {
		printInfo("reading stdin — paste or type, then Ctrl-D to send (Ctrl-C cancels)")
	}
	data, err := readAllStdin()
	if err != nil {
		printError(err.Error())
		return 1
	}
	if len(data) == 0 {
		printError("nothing on stdin — nothing to upload")
		return 1
	}
	return uploadStdin(f, bytes.NewReader(data))
}

// runWcpc reads the local clipboard and uploads it — it works against
// any backend (same as -v/--paste), unlike wcpp/wcpv below.
func runWcpc(args []string) int {
	f, err := ParseArgs(args)
	if err != nil {
		printError(err.Error())
		return 1
	}
	if f.Help {
		fmt.Print(Usage())
		return 0
	}
	return runPasteUpload(f)
}

// runWcpp needs the self-hosted wcp-host backend's rotating index,
// which doesn't exist until the server (build order step 5) is
// deployed — stubbed rather than faked against catbox/litterbox.
func runWcpp() int {
	printError("wcpp needs the self-hosted wcp-host backend, not deployed yet")
	return 1
}

// runWcpv has two shapes. With a code, it's a generic retrieval — works
// against any backend, same as `wcp <code>` — except a plain-text
// result goes to the clipboard instead of stdout; anything else (a
// real file, carrying a wcp-name header) is saved exactly like `wcp
// <code>` would. With no code, it falls back to pulling the
// self-hosted rotating index's latest entry, which (like wcpp) needs a
// server that doesn't exist yet.
func runWcpv(args []string) int {
	f, err := ParseArgs(args)
	if err != nil {
		printError(err.Error())
		return 1
	}
	if f.Help {
		fmt.Print(Usage())
		return 0
	}
	if len(f.Args) > 0 {
		return explicitRetrieve(f, f.Args[0], "", f.OutFile, clipboardSink)
	}
	printError("wcpv with no code needs the self-hosted wcp-host backend, not deployed yet")
	return 1
}
