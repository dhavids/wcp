package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// doUpdate copies this binary over the one already on PATH, matching
// run_update's three guard rails: refuse if nothing is installed yet,
// refuse if this is already the installed copy, and skip the copy
// (without error) if the two are byte-identical.
func doUpdate() int {
	srcPath, err := resolvedExecutable()
	if err != nil {
		printError(err.Error())
		return 1
	}

	installed, err := exec.LookPath("wcp")
	if err != nil {
		printError("wcp is not on your PATH yet — install it first")
		return 1
	}
	installedReal, err := filepath.EvalSymlinks(installed)
	if err != nil {
		printError(err.Error())
		return 1
	}

	if installedReal == srcPath {
		printError("already running the installed copy — run --update from the repo")
		printError("example: cd /path/to/wcp-repo && ./wcp --update")
		return 1
	}

	same, err := filesEqual(srcPath, installedReal)
	if err != nil {
		printError(err.Error())
		return 1
	}
	if same {
		printInfo("already up to date: " + installedReal)
		return 0
	}

	if err := copyExecutable(srcPath, installedReal); err != nil {
		printError(err.Error())
		return 1
	}
	printInfo("updated: " + installedReal)
	return 0
}

// Helpers

func resolvedExecutable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("could not locate the running binary: %w", err)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("could not resolve %s: %w", path, err)
	}
	return real, nil
}

func filesEqual(a, b string) (bool, error) {
	da, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	db, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(da, db), nil
}

func copyExecutable(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}
