// Package shell quotes words for a shell, so that a command a message
// gives reads as itself when it is pasted: in a POSIX shell, and in zsh,
// macOS's default shell. It doesn't import testing.
package shell

import "strings"

// Quote returns s as one word of a shell command: as it is when it is not
// empty, each of its bytes is one no shell reads specially, and its first
// isn't =, which zsh reads as a command to look up; and otherwise in
// single quotes, with each single quote in it written as
//
//	'\''
//
// It quotes for a shell with its default options. zsh with
// magicequalsubst set, which is off by default, reads x==ls, which Quote
// leaves as it is, as x= and the path of ls.
func Quote(s string) string {
	if s != "" && s[0] != '=' && strings.IndexFunc(s, special) < 0 {
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
