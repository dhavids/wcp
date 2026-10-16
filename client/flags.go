package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"wcp/utils/code"
	"wcp/utils/config"
)

// Flags holds every parsed command-line option plus the leftover
// positional arguments, mirroring the bash client's global flag set.
type Flags struct {
	Backend      code.Backend
	Host         string
	LBHours      int
	Copy         bool
	NoCopy       bool
	OutFile      string
	KeyLen       int
	Paste        bool
	Help         bool
	Encrypt      bool
	Plain        bool
	Link         bool
	Force        bool
	Accumulate   bool
	Send         bool
	ClearBuffer  bool
	Full         bool
	Update       bool
	NoStoredKey  bool
	Permanent    bool
	DownloadDir  string
	HotkeyDaemon bool
	Args         []string
}

const usageText = `wcp — upload a file, some text, or stdin; get back a short code.

Usage:
  wcp <file>                     upload a file
  wcp some words here            upload text
  echo text | wcp                upload stdin
  wcp                            read stdin; use this for MULTI-LINE text
  wcp <code>                     retrieve; falls back to uploading on a miss
  wcp . <code|url>               retrieve; errors on a miss
  wcp <code|url> -o <file>       retrieve and save under a chosen name

Options:
  -b, --backend <name>   catbox (default) or litterbox; -b c / -b l also work
      --host <url>       override the backend endpoint, or the fetch host
  -t, --time <hours>     litterbox expiry, rounds up to 1/12/24/72 (default 1)
  -e, --encrypt          force client-side encryption
  -u, --plain            force no encryption
  -p, --permanent        store permanently (needs the self-hosted backend)
  -c, --copy             copy the code even when output is redirected
  -n, --no-copy          do not copy the code to the clipboard
  -k, --key-len <n>      encryption key length, 12 to 64 (default 12)
  -l, --link             print the full URL instead of the short code
  -f, --force            encrypt a file of any type, up to 100 MB
  -a, --accumulate       append stdin, text or the clipboard to the buffer
                         with nothing to append, shows the buffer instead
  -s, --send             upload the accumulated buffer and clear it
      --clear            empty the buffer without uploading
      --full             with -a, show the whole buffer, not head and tail
      --update           copy this repo's wcp over the installed one on PATH
                         run from the repo, e.g. ./wcp --update, after a pull
  -v, --paste            upload the clipboard contents (takes no arguments)
  -z, --no-stored-key    ignore the stored key for this run
  -d, --download-dir <dir>  save a retrieved file here instead of the
                         default (created if missing; falls back to the
                         default and prints that path if it can't be
                         created)
  -h, --help             show this help

Identity (no flags of their own):
  wcp set key [key]      store a key (20+ chars); with no value, generate one
  wcp get key            print the stored key
  wcp clear key          forget the stored key
  wcp set user <name>    store a username (for the self-hosted backend)
  wcp get user           print the stored username
  wcp clear user         forget the stored username
  wcp set crid <value>   store a <username>-<key> pair in one call
  wcp get crid           print username and key as <username>-<key>
  wcp clear crid         forget both username and key
  wcp set download-path <dir>   always save retrieved files here
  wcp get download-path         print the stored path (or the default)
  wcp clear download-path       forget it, back to the default
  wcp set hotkey copy|paste on [combo]   enable a global hotkey (default
                         ctrl+alt+c for copy, ctrl+alt+v for paste); copy
                         uploads the clipboard, paste needs a self-hosted
                         backend
  wcp set hotkey copy|paste off          disable it
  wcp get hotkey         print both actions' on/off state and combo
  wcp clear hotkey       reset both combos to their defaults
  wcp get history [n]    show the last n logged commands (default 5)

Env: WCP_BACKEND, WCP_TIME (whole hours), WCP_PLAIN=1, WCP_KEY_LEN, WCP_NO_COPY=1
`

// Public functions

// ParseArgs reads flags out of args, leaving positional arguments in
// Flags.Args. Env var defaults are applied before any flag is read, so a
// flag always wins over its matching env var.
func ParseArgs(args []string) (*Flags, error) {
	f := &Flags{
		Backend: envBackend(),
		LBHours: envInt("WCP_TIME", 1),
		NoCopy:  os.Getenv("WCP_NO_COPY") == "1",
		KeyLen:  envInt("WCP_KEY_LEN", config.DefaultKeyLen),
	}

	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			f.Args = append(f.Args, args[i+1:]...)
			break
		}

		name, inlineVal, hasInline := splitInline(arg)
		next := func() (string, error) {
			if hasInline {
				return inlineVal, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", arg)
			}
			i++
			return args[i], nil
		}

		switch name {
		case "-b", "--backend":
			v, err := next()
			if err != nil {
				return nil, err
			}
			f.Backend = normalizeBackend(v)
		case "--host":
			v, err := next()
			if err != nil {
				return nil, err
			}
			f.Host = v
		case "-t", "--time":
			v, err := next()
			if err != nil {
				return nil, err
			}
			n, convErr := strconv.Atoi(strings.TrimSuffix(v, "h"))
			if convErr != nil {
				return nil, fmt.Errorf("-t takes a whole number of hours, 1 to 72")
			}
			f.LBHours = n
		case "-c", "--copy":
			f.Copy = true
		case "-n", "--no-copy":
			f.NoCopy = true
		case "-o", "--output":
			v, err := next()
			if err != nil {
				return nil, err
			}
			f.OutFile = v
		case "-d", "--download-dir":
			v, err := next()
			if err != nil {
				return nil, err
			}
			f.DownloadDir = v
		case "-k", "--key-len":
			v, err := next()
			if err != nil {
				return nil, err
			}
			n, convErr := strconv.Atoi(v)
			if convErr != nil {
				return nil, fmt.Errorf("--key-len must be a whole number from 12 to 64")
			}
			f.KeyLen = n
		case "-v", "--paste":
			f.Paste = true
		case "-h", "--help":
			f.Help = true
		case "-e", "--encrypt":
			f.Encrypt = true
		case "-u", "--plain":
			f.Plain = true
		case "-p", "--permanent":
			f.Permanent = true
		case "-l", "--link":
			f.Link = true
		case "-f", "--force":
			f.Force = true
		case "-a", "--accumulate":
			f.Accumulate = true
		case "-s", "--send":
			f.Send = true
		case "--clear":
			f.ClearBuffer = true
		case "--full":
			f.Full = true
		case "--update":
			f.Update = true
		case "--hotkey-daemon":
			// Internal: launched by the hotkey daemon's own autostart
			// entry/process spawn, not a user-facing flag (not in Usage).
			f.HotkeyDaemon = true
		case "-z", "--no-stored-key":
			f.NoStoredKey = true
		default:
			f.Args = append(f.Args, arg)
		}
		i++
	}

	return f, nil
}

// Usage returns the help text shown for -h/--help.
func Usage() string {
	return usageText
}

// Helpers

// splitInline splits "--flag=value" into ("--flag", "value", true); any
// other argument comes back as (arg, "", false).
func splitInline(arg string) (name, value string, ok bool) {
	if i := strings.Index(arg, "="); i > 0 && strings.HasPrefix(arg, "-") {
		return arg[:i], arg[i+1:], true
	}
	return arg, "", false
}

func normalizeBackend(v string) code.Backend {
	switch v {
	case "c", "catbox":
		return code.Catbox
	case "l", "litterbox":
		return code.Litterbox
	default:
		return code.Backend(v)
	}
}

func envBackend() code.Backend {
	if v := os.Getenv("WCP_BACKEND"); v != "" {
		return normalizeBackend(v)
	}
	return code.Catbox
}

func envInt(name string, fallback int) int {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
