package decl_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/erase"
	"github.com/sdboyer/jenny"
)

// TestReject: a bytes lift's jenny's error is a rejection only when Reject
// marks all of it, through every error it wraps or joins, so a rejection
// joined with a fault, as jenny's Pipeline joins its jennies' errors, is a
// fault, and a join of rejections, as jenny joins one jenny's error alone,
// is a rejection. Reject marks no nil error.
func TestReject(t *testing.T) {
	if err := decl.Reject(nil); err != nil {
		t.Errorf("Reject(nil) = %#v, want nil", err)
	}
	rej := func(text string) error { return decl.Reject(errors.New(text)) }
	gen := func(name string, err error) jenny.Jenny[string] {
		return jenny.For[string]().New(name, func(string) (jenny.Files, error) { return nil, err })
	}
	pipeline := func(js ...jenny.Jenny[string]) error {
		_, err := jenny.Pipeline(js...).Generate("in")
		return err
	}
	one := func(j jenny.Jenny[string]) error {
		_, err := j.Generate("in")
		return err
	}
	fault := errors.New("a fault")
	for _, tt := range []struct {
		name string
		err  error
		want bool
	}{
		{"a rejection", rej("unclosed quote"), true},
		{"a wrapped rejection", fmt.Errorf("import: %w", rej("unclosed quote")), true},
		{"an unmarked error", fault, false},
		{"a wrapped unmarked error", fmt.Errorf("import: %w", fault), false},
		{"a rejection joined alone", errors.Join(rej("unclosed quote")), true},
		{"joined rejections", errors.Join(rej("unclosed quote"), rej("stray quote")), true},
		{"a rejection joined with a fault", errors.Join(rej("unclosed quote"), fault), false},
		{"a fault joined with a rejection", errors.Join(fault, rej("unclosed quote")), false},
		{"a rejection and a fault in one Errorf", fmt.Errorf("%w; %w", rej("unclosed quote"), fault), false},
		{"a rejection of joined errors", decl.Reject(errors.Join(fault, errors.New("another"))), true},
		{"a jenny's rejection, which it joins alone", one(gen("a", rej("unclosed quote"))), true},
		{"a jenny's fault, which it joins alone", one(gen("a", fault)), false},
		{"a Pipeline of two rejections", pipeline(gen("a", rej("unclosed quote")), gen("b", rej("stray quote"))), true},
		{"a Pipeline of a rejection and a fault", pipeline(gen("a", rej("unclosed quote")), gen("b", fault)), false},
		{"a Pipeline of a fault and a rejection", pipeline(gen("a", fault), gen("b", rej("unclosed quote"))), false},
	} {
		if got := erase.IsRejection(tt.err); got != tt.want {
			t.Errorf("%s: IsRejection(%q) = %v, want %v", tt.name, tt.err, got, tt.want)
		}
	}
}
