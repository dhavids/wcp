package main

import (
	"fmt"
	"os"
	"path/filepath"

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

// resolveDownloadDir picks where a saved file should land: -d wins,
// then the stored `download-path` setting, then the platform default
// (DefaultDownloadDir). If the chosen directory can't be created, it
// falls back to the platform default and uses that instead — a save
// should never just fail because a custom path went stale.
func resolveDownloadDir(f *Flags) string {
	dir := f.DownloadDir
	if dir == "" {
		dir, _ = config.ReadDownloadPath()
	}
	if dir == "" {
		return config.DefaultDownloadDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return config.DefaultDownloadDir()
	}
	return dir
}

// saveUnderDownloadDir saves data as name inside the resolved
// download directory, returning the actual path used — which is what
// gets printed, since a fallback may have changed it.
func saveUnderDownloadDir(f *Flags, name string, data []byte) (string, error) {
	dir := resolveDownloadDir(f)
	outName := uniqueSaveName(filepath.Join(dir, name))
	if err := os.WriteFile(outName, data, 0o644); err != nil {
		return "", err
	}
	return outName, nil
}
