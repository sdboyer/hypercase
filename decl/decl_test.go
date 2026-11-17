package decl_test

import (
	"errors"
	"fmt"
	"strings"
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
	// dup makes two files at one path, which jenny refuses.
	dup := jenny.For[string]().New("dup", func(string) (jenny.Files, error) {
		return jenny.Files{jenny.NewFile("x", []byte("x\n")), jenny.NewFile("x", []byte("x\n"))}, nil
	})
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
		// Reject marks all of what it is given, jenny's own faults in it
		// included.
		{"a generator's rejection of an inner jenny's fault", one(gen("a", decl.Reject(one(dup)))), true},
		// The walk follows Unwrap alone, never an error's own As.
		{"a chain of two rejections, as go-multierror holds them", chain{rej("unclosed quote"), rej("stray quote")}, false},
		{"that chain, marked whole", decl.Reject(chain{rej("unclosed quote"), rej("stray quote")}), true},
		{"a rejection reached through 100 Unwrap calls", wraps(100, rej("unclosed quote")), true},
		{"a rejection reached through 101 Unwrap calls", wraps(101, rej("unclosed quote")), false},
		{"an error whose Unwrap gives itself", &cyclic{}, false},
		{"a join that holds a rejection and itself", &cyclicJoin{rej("unclosed quote")}, false},
	} {
		if got := erase.IsRejection(tt.err); got != tt.want {
			t.Errorf("%s: IsRejection(%q) = %v, want %v", tt.name, tt.err, got, tt.want)
		}
	}
	if err := one(dup); err == nil || !strings.Contains(err.Error(), "multiple files at path x") {
		t.Errorf("the inner jenny's error is %v, want jenny's own fault, two files at one path", err)
	}
	if !errors.As(chain{decl.Reject(&codedError{"C101"}), rej("stray quote")}, new(*codedError)) {
		t.Errorf("the chain's As doesn't reach its first error, as go-multierror's does")
	}
}

// chain is shaped as go-multierror's chain of errors: its text and its As
// and Is are its first error's, and its Unwrap gives the chain of the rest,
// or nil for the last.
type chain []error

func (c chain) Error() string { return c[0].Error() }
func (c chain) Unwrap() error {
	if len(c) == 1 {
		return nil
	}
	return c[1:]
}
func (c chain) As(target any) bool   { return errors.As(c[0], target) }
func (c chain) Is(target error) bool { return errors.Is(c[0], target) }

type codedError struct{ code string }

func (e *codedError) Error() string { return "an error with the code " + e.code }

// wraps wraps err in n errors, each with Unwrap() error.
func wraps(n int, err error) error {
	for range n {
		err = fmt.Errorf("w: %w", err)
	}
	return err
}

// cyclic's Unwrap gives itself, and cyclicJoin's gives its error and
// itself.
type (
	cyclic     struct{}
	cyclicJoin struct{ err error }
)

func (e *cyclic) Error() string       { return "a cycle" }
func (e *cyclic) Unwrap() error       { return e }
func (e *cyclicJoin) Error() string   { return "a cycle through a join" }
func (e *cyclicJoin) Unwrap() []error { return []error{e.err, e} }
