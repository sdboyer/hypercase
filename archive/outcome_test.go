package archive

import (
	"reflect"
	"strings"
	"testing"
)

// TestOutcomeFormat: every outcome an outcome file holds reads back as
// itself, and Format writes it again byte for byte.
func TestOutcomeFormat(t *testing.T) {
	for _, tt := range []struct {
		data string
		want Outcome
	}{
		{"accepted\n", Outcome{State: Accepted}},
		{"accepted div-zero\n", Outcome{State: Accepted, Detail: "div-zero"}},
		{"accepted two words; and more\n", Outcome{State: Accepted, Detail: "two words; and more"}},
		{"rejected\n", Outcome{State: Rejected}},
		{"rejected S201\n", Outcome{State: Rejected, Codes: []string{"S201"}}},
		{"rejected S101 S201\n", Outcome{State: Rejected, Codes: []string{"S101", "S201"}}},
		{"rejected S201; div-zero\n", Outcome{State: Rejected, Codes: []string{"S201"}, Detail: "div-zero"}},
		{"rejected; div-zero; and more\n", Outcome{State: Rejected, Detail: "div-zero; and more"}},
		{"panicked\n", Outcome{State: Panicked}},
		{"blocked resolve\n", Outcome{State: Blocked, Detail: "resolve"}},
		{"skipped no formulas\n", Outcome{State: Skipped, Detail: "no formulas"}},
	} {
		got, err := ParseOutcome([]byte(tt.data))
		if err != nil {
			t.Errorf("ParseOutcome(%q): %v", tt.data, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseOutcome(%q) = %+v, want %+v", tt.data, got, tt.want)
		}
		if f := string(tt.want.Format()); f != tt.data {
			t.Errorf("Format of %+v = %q, want %q", tt.want, f, tt.data)
		}
	}
}

// TestOutcomeRefused: an outcome file holds one form of each outcome, and
// nothing else.
func TestOutcomeRefused(t *testing.T) {
	for _, tt := range []struct{ data, want string }{
		{"accepted", "one line, ending with a newline"},
		{"accepted\nrejected\n", "one line, ending with a newline"},
		{"accepted\r\n", "one line, ending with a newline"},
		{"\n", `one of accepted, rejected, panicked, blocked, skipped, not ""`},
		{"passed\n", `not "passed"`},
		{"Accepted\n", `not "Accepted"`},
		{"accepted \n", `it reads as "accepted"`},
		{"accepted  two\n", `" two" can't be held in an outcome`},
		{"accepted two \n", `"two " can't be held in an outcome`},
		{"rejected S201 S101\n", "S101 follows S201"},
		{"rejected S201 S201\n", "S201 follows S201"},
		{"rejected  S201\n", `the code "" can't be held`},
		{"rejected S201;\n", `the code "S201;" can't be held`},
		{"rejected;\n", "rejected is followed by its codes"},
		{"rejected ; x\n", `the code "" can't be held`},
		{"rejected S201;  x\n", `" x" can't be held`},
		{"rejectedS201\n", `not "rejectedS201"`},
		{"panicked now\n", "panicked stands alone"},
		{"blocked\n", "blocked is followed by the lift that failed"},
		{"blocked Resolve\n", `"Resolve" is no lift's name`},
		{"blocked resolve parse\n", `"resolve parse" is no lift's name`},
		{"skipped\n", "skipped is followed by the probe's reason"},
	} {
		_, err := ParseOutcome([]byte(tt.data))
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("ParseOutcome(%q) = %v, want an error holding %q", tt.data, err, tt.want)
		}
	}
	if err := (Outcome{State: Accepted, Codes: []string{"S1"}}).Check(); err == nil || !strings.Contains(err.Error(), "only a rejected outcome has codes") {
		t.Errorf("an accepted outcome with codes: %v", err)
	}
}
