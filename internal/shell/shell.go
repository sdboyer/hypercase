// Package shell quotes words for a POSIX shell, so that a command a
// message gives runs as it reads when it is pasted. It doesn't import
// testing.
package shell

import "strings"

// Quote returns s as one word of a POSIX shell command: as it is when it
// is not empty and each of its bytes is one no shell reads specially, and
// otherwise in single quotes, with each single quote in it written as
//
//	'\''
func Quote(s string) string {
	if s != "" && strings.IndexFunc(s, special) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// special reports whether a shell may read r as other than itself.
func special(r rune) bool {
	switch {
	case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', '0' <= r && r <= '9':
		return false
	}
	return !strings.ContainsRune("%+,-./:=@_", r)
}
