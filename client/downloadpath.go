package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wcp/utils/config"
)

// runDownloadPathVerb handles `wcp get/set/clear download-path`.
func runDownloadPathVerb(verb string, rest []string) int {
	switch verb {
	case "set":
		if len(rest) != 1 {
			printError("set download-path needs exactly one directory")
			return 1
		}
		return doSetDownloadPath(rest[0])
	case "get":
		if len(rest) > 0 {
			printError("get download-path takes no arguments")
			return 1
		}
		return doGetDownloadPath()
	default:
		if len(rest) > 0 {
			printError("clear download-path takes no arguments")
			return 1
		}
		return doClearDownloadPath()
	}
}

func doSetDownloadPath(dir string) int {
	if err := config.WriteDownloadPath(dir); err != nil {
		printError(err.Error())
		return 1
	}
	printInfo("download path set to " + dir)
	return 0
}

func doGetDownloadPath() int {
	dir, err := config.ReadDownloadPath()
	if err != nil {
		printError(err.Error())
		return 1
	}
	if dir == "" {
		dir = config.DefaultDownloadDir()
	}
	fmt.Println(dir)
	return 0
}

func doClearDownloadPath() int {
	if err := config.DeleteDownloadPath(); err != nil {
		printError(err.Error())
		return 1
	}
	printInfo("download path cleared, back to the default (" + config.DefaultDownloadDir() + ")")
	return 0
}

// splitDownloadOverride splits -d's argument into a directory and an
// optional filename: -d can be a bare directory (existing, or ending
// in a separator), or a path with a filename at the end (e.g.
// -d ~/stuff/report.pdf), in which case that filename wins over the
// retrieval's own name.
func splitDownloadOverride(path string) (dir, name string) {
	if path == "" {
		return "", ""
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path, ""
	}
	if strings.HasSuffix(path, string(filepath.Separator)) {
		return strings.TrimSuffix(path, string(filepath.Separator)), ""
	}
	return filepath.Dir(path), filepath.Base(path)
}

// resolveSavePath picks exactly where a retrieved file should land,
// given its own name (from a wcp-name header, or a plain retrieval's
// URL basename): -d's directory wins, then the stored `download-path`
// setting, then the platform default — and if the chosen directory
// can't be created, falls back to the platform default instead, so a
// save never just fails because a custom path went stale. The
// filename is -d's own embedded name if it had one, else -o, else
// originalName.
func resolveSavePath(f *Flags, originalName string) string {
	dir, dirName := "", ""
	if f.DownloadDir != "" {
		dir, dirName = splitDownloadOverride(f.DownloadDir)
	}
	if dir == "" {
		dir, _ = config.ReadDownloadPath()
	}
	if dir == "" {
		dir = config.DefaultDownloadDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		dir = config.DefaultDownloadDir()
	}

	name := originalName
	if f.OutFile != "" {
		name = f.OutFile
	}
	if dirName != "" {
		name = dirName
	}
	return uniqueSaveName(filepath.Join(dir, name))
}

// saveUnderDownloadDir saves data under name, resolved via
// resolveSavePath, returning the actual path used — which is what
// gets printed, since an override or a fallback may have changed it.
func saveUnderDownloadDir(f *Flags, name string, data []byte) (string, error) {
	outPath := resolveSavePath(f, name)
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return "", err
	}
	return outPath, nil
}
