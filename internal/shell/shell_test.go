package shell

import (
	"os/exec"
	"testing"
)

// TestQuote: a word that needs no quotes is left as it is, and every word,
// quoted, reads back as itself in a POSIX shell, a space, a quote, a
// newline and the shell's own characters among them. It runs sh, which a
// POSIX system has, so it skips where there is none.
func TestQuote(t *testing.T) {
	for _, s := range []string{"corp", "../a/b", "walkthrough/step05", "-root", "x=1,y:2@%+_"} {
		if q := Quote(s); q != s {
			t.Errorf("Quote(%q) = %q, and should leave it as it is", s, q)
		}
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("no sh to read the words back: %v", err)
	}
	for _, s := range []string{"", "corp", "my corp", "../my corp", "it's", `a"b`, `a\b`, "$HOME", "`date`", "*", "~", "a;b", "a|b&c", "(x)", "#x", "tab\there", "new\nline", "''"} {
		q := Quote(s)
		out, err := exec.Command(sh, "-c", "printf %s "+q).Output()
		if err != nil {
			t.Errorf("sh -c 'printf %%s %s': %v", q, err)
			continue
		}
		if string(out) != s {
			t.Errorf("Quote(%q) = %s, which sh reads as %q", s, q, out)
		}
	}
}
