package shell

import (
	"os/exec"
	"testing"
)

// TestQuote: a word that needs no quotes is left as it is, and every
// other word is quoted as each of Quote's rules says, checked by the word
// it returns, which needs no shell: the empty word, a leading =, which zsh
// reads as a command to look up, a byte a shell reads specially,
// non-ASCII and invalid UTF-8 among them, and a single quote, which ends
// the quotes, is escaped, and reopens them. Then every word, quoted, reads
// back as itself, one argument, in a POSIX shell and in zsh. That half
// runs sh, which a POSIX system has, and zsh with no startup files, and
// skips the half of a shell that isn't there, saying why.
func TestQuote(t *testing.T) {
	plain := []string{"corp", "../a/b", "walkthrough/step05", "-root", "x=1,y:2@%+_", "a=b", "%x", "+x", "@x", ",x", ":x"}
	for _, s := range plain {
		if q := Quote(s); q != s {
			t.Errorf("Quote(%q) = %q, and should leave it as it is", s, q)
		}
	}
	quoted := []struct{ s, want string }{
		{"", "''"},
		{"=corp", "'=corp'"},
		{"=", "'='"},
		{"my corp", "'my corp'"},
		{"../my corp", "'../my corp'"},
		{`a"b`, `'a"b'`},
		{`a\b`, `'a\b'`},
		{"$HOME", "'$HOME'"},
		{"`date`", "'`date`'"},
		{"*", "'*'"},
		{"~", "'~'"},
		{"a;b", "'a;b'"},
		{"a|b&c", "'a|b&c'"},
		{"(x)", "'(x)'"},
		{"#x", "'#x'"},
		{"tab\there", "'tab\there'"},
		{"new\nline", "'new\nline'"},
		{"café", "'café'"},
		{"日本", "'日本'"},
		{"\xff", "'\xff'"},
		{"a\xffb", "'a\xffb'"},
		{"it's", `'it'\''s'`},
		{"''", `''\'''\'''`},
	}
	words := append([]string(nil), plain...)
	for _, w := range quoted {
		if q := Quote(w.s); q != w.want {
			t.Errorf("Quote(%q) = %q, and should be %q", w.s, q, w.want)
		}
		words = append(words, w.s)
	}
	for _, sh := range []struct {
		name  string
		flags []string
	}{{"sh", nil}, {"zsh", []string{"-f"}}} {
		t.Run(sh.name, func(t *testing.T) {
			path, err := exec.LookPath(sh.name)
			if err != nil {
				t.Skipf("no %s to read the words back: %v", sh.name, err)
			}
			for _, s := range words {
				q := Quote(s)
				script := "set -- " + q + `; printf '%s\n' "$#"; for a; do printf '[%s]' "$a"; done`
				out, err := exec.Command(path, append(sh.flags, "-c", script)...).Output()
				if err != nil {
					t.Errorf("%s -c %q: %v", sh.name, script, err)
					continue
				}
				if want := "1\n[" + s + "]"; string(out) != want {
					t.Errorf("Quote(%q) = %s, which %s reads as %q, not the one argument %q", s, q, sh.name, out, want)
				}
			}
		})
	}
}
