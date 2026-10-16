package main

import (
	"fmt"
	"strings"

	"wcp/utils/config"
	"wcp/utils/rtlog"
)

// verbTargets lists the identity/state nouns `wcp get/set/clear` accept.
var verbTargets = map[string]bool{"key": true, "user": true, "crid": true, "hotkey": true}

// isVerbCommand reports whether args looks like `wcp get|set|clear
// key|user|crid|hotkey ...` or `wcp get history [n]` — checked before
// flag parsing, since these verbs take no flags of their own.
func isVerbCommand(args []string) bool {
	if len(args) < 2 {
		return false
	}
	if args[0] == "get" && args[1] == "history" {
		return true
	}
	switch args[0] {
	case "get", "set", "clear":
		return verbTargets[args[1]]
	}
	return false
}

// runVerb dispatches an identity verb command to its target's handler.
// It isn't itself logged — each handler logs its own, more specific
// cmd name (e.g. "set key" vs. just "verb").
func runVerb(args []string) int {
	verb, target, rest := args[0], args[1], args[2:]
	cmd := verb + " " + target
	if verb == "get" && target == "history" {
		return rtlog.Track(entryPoint(), cmd, args, func() int { return doGetHistory(rest) })
	}
	switch target {
	case "key":
		return rtlog.Track(entryPoint(), cmd, args, func() int { return runKeyVerb(verb, rest) })
	case "user":
		return rtlog.Track(entryPoint(), cmd, args, func() int { return runUserVerb(verb, rest) })
	case "crid":
		return rtlog.Track(entryPoint(), cmd, args, func() int { return runCridVerb(verb, rest) })
	default:
		return rtlog.Track(entryPoint(), cmd, args, func() int { return runHotkeyVerb(verb, rest) })
	}
}

// Helpers

func runKeyVerb(verb string, rest []string) int {
	switch verb {
	case "set":
		if len(rest) > 1 {
			printError("set key takes at most one value")
			return 1
		}
		value := ""
		if len(rest) == 1 {
			value = rest[0]
		}
		return doSetKey(value)
	case "get":
		if len(rest) > 0 {
			printError("get key takes no arguments")
			return 1
		}
		return doGetKey()
	default:
		if len(rest) > 0 {
			printError("clear key takes no arguments")
			return 1
		}
		return doClearKey()
	}
}

func runUserVerb(verb string, rest []string) int {
	switch verb {
	case "set":
		if len(rest) != 1 {
			printError("set user needs exactly one username")
			return 1
		}
		return doSetUser(rest[0])
	case "get":
		if len(rest) > 0 {
			printError("get user takes no arguments")
			return 1
		}
		return doGetUser()
	default:
		if len(rest) > 0 {
			printError("clear user takes no arguments")
			return 1
		}
		return doClearUser()
	}
}

func runCridVerb(verb string, rest []string) int {
	switch verb {
	case "set":
		if len(rest) != 1 {
			printError("set crid needs exactly one <username>-<key> value")
			return 1
		}
		return doSetCrid(rest[0])
	case "get":
		if len(rest) > 0 {
			printError("get crid takes no arguments")
			return 1
		}
		return doGetCrid()
	default:
		if len(rest) > 0 {
			printError("clear crid takes no arguments")
			return 1
		}
		return doClearCrid()
	}
}

func doSetKey(value string) int {
	if value == "" {
		rest, err := config.GenKey(config.MinStoredKeyLen - 1)
		if err != nil {
			printError(err.Error())
			return 1
		}
		key := "A" + rest
		if err := config.WriteStoredKey(key); err != nil {
			printError(err.Error())
			return 1
		}
		printInfo("generated a new key and stored it")
		fmt.Println(key)
		return 0
	}
	if err := config.WriteStoredKey(value); err != nil {
		printError(err.Error())
		return 1
	}
	printInfo("key stored")
	return 0
}

func doGetKey() int {
	key, err := config.ReadStoredKey()
	if err != nil {
		printError(err.Error())
		return 1
	}
	if key == "" {
		printError("no key is set on this machine")
		return 1
	}
	fmt.Println(key)
	return 0
}

func doClearKey() int {
	existed, err := config.HasStoredKey()
	if err != nil {
		printError(err.Error())
		return 1
	}
	if err := config.DeleteStoredKey(); err != nil {
		printError(err.Error())
		return 1
	}
	if existed {
		printInfo("key cleared")
	} else {
		printInfo("no key was set")
	}
	return 0
}

// doSetUser stores username locally after validating its charset.
//
// TODO(server): the design calls for a round trip to the self-hosted
// backend first ("does this user exist? if not, add it with swcp
// add-user first") — not wired up yet since the server doesn't exist.
// Stores locally in the meantime and says so, so this isn't silently
// weaker than documented.
func doSetUser(username string) int {
	if err := config.ValidateUsername(username); err != nil {
		printError(err.Error())
		return 1
	}
	if err := config.WriteStoredUser(username); err != nil {
		printError(err.Error())
		return 1
	}
	printInfo("user stored (not yet verified against a server — none deployed)")
	return 0
}

func doGetUser() int {
	user, err := config.ReadStoredUser()
	if err != nil {
		printError(err.Error())
		return 1
	}
	if user == "" {
		printError("no user is set on this machine")
		return 1
	}
	fmt.Println(user)
	return 0
}

func doClearUser() int {
	existed, err := config.HasStoredUser()
	if err != nil {
		printError(err.Error())
		return 1
	}
	if err := config.DeleteStoredUser(); err != nil {
		printError(err.Error())
		return 1
	}
	if existed {
		printInfo("user cleared")
	} else {
		printInfo("no user was set")
	}
	return 0
}

func doGetCrid() int {
	user, err := config.ReadStoredUser()
	if err != nil {
		printError(err.Error())
		return 1
	}
	key, err := config.ReadStoredKey()
	if err != nil {
		printError(err.Error())
		return 1
	}
	if user == "" || key == "" {
		printError("crid needs both a user and a key set — see wcp get user / wcp get key")
		return 1
	}
	fmt.Println(formatCrid(user, key))
	return 0
}

// doSetCrid consumes a `<username>-<key>` string as printed by `get
// crid` on another machine, storing both in one call.
func doSetCrid(value string) int {
	username, key, err := parseCrid(value)
	if err != nil {
		printError(err.Error())
		return 1
	}
	if err := config.ValidateUsername(username); err != nil {
		printError(err.Error())
		return 1
	}
	if err := config.WriteStoredUser(username); err != nil {
		printError(err.Error())
		return 1
	}
	if err := config.WriteStoredKey(key); err != nil {
		printError(err.Error())
		return 1
	}
	printInfo("user and key stored (user not yet verified against a server — none deployed)")
	return 0
}

func doClearCrid() int {
	hadUser, err := config.HasStoredUser()
	if err != nil {
		printError(err.Error())
		return 1
	}
	hadKey, err := config.HasStoredKey()
	if err != nil {
		printError(err.Error())
		return 1
	}
	if err := config.DeleteStoredUser(); err != nil {
		printError(err.Error())
		return 1
	}
	if err := config.DeleteStoredKey(); err != nil {
		printError(err.Error())
		return 1
	}
	if hadUser || hadKey {
		printInfo("crid cleared")
	} else {
		printInfo("no crid was set")
	}
	return 0
}

// formatCrid joins username and key the way doGetCrid prints them and
// doSetCrid/parseCrid expect to read them back.
func formatCrid(username, key string) string {
	return username + "-" + key
}

// parseCrid splits a `<username>-<key>` string. Keys always start with
// a literal "A" (config.GenKey prepends it), so it looks for the first
// "-A" rather than the first "-" — a username can't produce that exact
// sequence itself, since its charset excludes dashes and uppercase.
func parseCrid(value string) (username, key string, err error) {
	i := strings.Index(value, "-A")
	if i < 0 {
		return "", "", fmt.Errorf("crid: expected <username>-<key> (key starting with A), got %q", value)
	}
	return value[:i], value[i+1:], nil
}
