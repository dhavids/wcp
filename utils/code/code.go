// Package code implements wcp's short-code format: a single prefix
// character that encodes both the backend and the file extension,
// followed by the server-assigned id.
package code

import "strings"

// Backend identifies which upload backend a code was issued by.
type Backend string

const (
	Litterbox Backend = "litterbox"
	Catbox    Backend = "catbox"
)

// extTable maps a prefix letter's position to an extension; index 0 is
// "no extension". Order is fixed — it's the wire format, not a lookup
// table that can be reordered.
var extTable = []string{
	"", "txt", "md", "log", "json", "yaml", "csv", "conf", "sh", "py",
	"png", "jpg", "pdf", "zip", "gz",
}

// Public functions

// EncodeID builds a short code from a server id and the backend it came
// from. "a-o" (litterbox) and "A-O" (catbox) carry a tracked extension in
// the letter itself; "0"/"1" escape to a literal ".ext" when the
// extension isn't in extTable.
func EncodeID(idFull string, backend Backend) string {
	base, ext := splitExt(idFull)
	for i, e := range extTable {
		if e == ext {
			return string(prefixLetter(i, backend)) + base
		}
	}
	if backend == Catbox {
		return "1" + base + "." + ext
	}
	return "0" + base + "." + ext
}

// DecodeCode reverses EncodeID. ok is false for an empty candidate or a
// prefix letter past the end of extTable.
func DecodeCode(candidate string) (backend Backend, id string, ok bool) {
	if candidate == "" {
		return "", "", false
	}
	letter := candidate[0]
	rest := candidate[1:]

	if letter == '0' || letter == '1' {
		if letter == '0' {
			return Litterbox, rest, true
		}
		return Catbox, rest, true
	}

	i, backend, ok := extIndex(letter)
	if !ok || i >= len(extTable) {
		return "", "", false
	}

	ext := extTable[i]
	if ext == "" {
		return backend, rest, true
	}
	return backend, rest + "." + ext, true
}

// Helpers

// splitExt splits on the last dot, matching EncodeID's own convention —
// the id side of a code may itself contain dots.
func splitExt(idFull string) (base, ext string) {
	i := strings.LastIndex(idFull, ".")
	if i < 0 {
		return idFull, ""
	}
	return idFull[:i], idFull[i+1:]
}

func prefixLetter(i int, backend Backend) byte {
	if backend == Catbox {
		return byte('A' + i)
	}
	return byte('a' + i)
}

func extIndex(letter byte) (i int, backend Backend, ok bool) {
	switch {
	case letter >= 'a' && letter <= 'z':
		return int(letter - 'a'), Litterbox, true
	case letter >= 'A' && letter <= 'Z':
		return int(letter - 'A'), Catbox, true
	default:
		return 0, "", false
	}
}
