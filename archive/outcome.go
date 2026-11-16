package archive

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// OutcomeFile is the name of a cell's outcome, in its column's directory:
// out/<column>/outcome, and the same under a variant's prefix.
const OutcomeFile = "outcome"

// The states of an outcome, its first word.
const (
	Accepted = "accepted"
	Rejected = "rejected"
	Panicked = "panicked"
	Blocked  = "blocked"
	Skipped  = "skipped"
)

// States are the states an outcome may have, in the order a reader meets
// them.
var States = []string{Accepted, Rejected, Panicked, Blocked, Skipped}

// An Outcome is the code's verdict on a cell's case, as its outcome file
// holds it.
type Outcome struct {
	// State is one of [States].
	State string
	// Codes are a rejected cell's codes, sorted byte by byte, each once.
	// Every other state has none.
	Codes []string
	// Detail is what follows the state: a classifier's detail, for an
	// accepted or a rejected cell; the lift that failed, for a blocked
	// one; the probe's reason, for a skipped one. A panicked cell has
	// none.
	Detail string
}

// Format returns the outcome file's bytes: one line, ending with a
// newline.
//
//	accepted
//	accepted <detail>
//	rejected
//	rejected <code> <code>
//	rejected <code>; <detail>
//	rejected; <detail>
//	panicked
//	blocked <lift>
//	skipped <reason>
func (o Outcome) Format() []byte {
	var b strings.Builder
	b.WriteString(o.State)
	for _, c := range o.Codes {
		b.WriteString(" " + c)
	}
	switch {
	case o.Detail == "":
	case o.State == Rejected:
		b.WriteString("; " + o.Detail)
	default:
		b.WriteString(" " + o.Detail)
	}
	b.WriteString("\n")
	return []byte(b.String())
}

// Check reports what makes the outcome one no outcome file holds. Each
// code holds no space, semicolon, carriage return or newline, and the
// codes are sorted, each once; the detail is one line with no space at
// either end; and each state has the parts its format gives it.
func (o Outcome) Check() error {
	if !slices.Contains(States, o.State) {
		return fmt.Errorf("an outcome starts with its state, one of %s, not %q", strings.Join(States, ", "), o.State)
	}
	if len(o.Codes) > 0 && o.State != Rejected {
		return fmt.Errorf("only a rejected outcome has codes, and this one is %s", o.State)
	}
	for i, c := range o.Codes {
		if err := CheckCode(c); err != nil {
			return err
		}
		if i > 0 && o.Codes[i-1] >= c {
			return fmt.Errorf("a rejected outcome's codes are sorted byte by byte, each once, and %s follows %s", c, o.Codes[i-1])
		}
	}
	if o.Detail != "" {
		if err := CheckDetail(o.Detail); err != nil {
			return err
		}
	}
	switch {
	case o.State == Panicked && o.Detail != "":
		return errors.New("panicked stands alone")
	case o.State == Blocked && o.Detail == "":
		return errors.New("blocked is followed by the lift that failed")
	case o.State == Blocked && !ValidName(o.Detail):
		return fmt.Errorf("blocked is followed by the lift that failed, and %q is no lift's name", o.Detail)
	case o.State == Skipped && o.Detail == "":
		return errors.New("skipped is followed by the probe's reason")
	}
	return nil
}

// CheckCode reports what makes a string no code an outcome holds: a code
// is not empty, and holds no space, semicolon, carriage return or newline.
func CheckCode(code string) error {
	if code == "" || strings.ContainsAny(code, " ;\r\n") {
		return fmt.Errorf("the code %q can't be held in an outcome: a code is not empty, and holds no space, semicolon, carriage return or newline", code)
	}
	return nil
}

// CheckDetail reports what makes a string no detail an outcome holds: a
// probe's reason or a classifier's detail is one line, not empty, with no
// space at either end.
func CheckDetail(text string) error {
	if text == "" || strings.ContainsAny(text, "\n\r") || strings.TrimSpace(text) != text {
		return fmt.Errorf("%q can't be held in an outcome: it is one line, not empty, with no space at either end", text)
	}
	return nil
}

// ParseOutcome reads an outcome file. It accepts only the bytes
// [Outcome.Format] writes, so an outcome reads one way.
func ParseOutcome(data []byte) (Outcome, error) {
	line, ok := bytes.CutSuffix(data, []byte("\n"))
	if !ok || bytes.ContainsAny(line, "\n\r") {
		return Outcome{}, errors.New("an outcome is one line, ending with a newline")
	}
	text := string(line)
	var o Outcome
	if rest, ok := strings.CutPrefix(text, Rejected); ok && (rest == "" || rest[0] == ' ' || rest[0] == ';') {
		o.State = Rejected
		codes, detail, _ := strings.Cut(rest, "; ")
		o.Detail = detail
		if codes != "" {
			if codes[0] != ' ' {
				return Outcome{}, fmt.Errorf("the outcome %q is not one an outcome file holds: rejected is followed by its codes, each after a space, then a semicolon and a space before a detail", text)
			}
			o.Codes = strings.Split(codes[1:], " ")
		}
	} else {
		o.State, o.Detail, _ = strings.Cut(text, " ")
	}
	if err := o.Check(); err != nil {
		return Outcome{}, err
	}
	if !bytes.Equal(o.Format(), data) {
		return Outcome{}, fmt.Errorf("the outcome %q is not one an outcome file holds: it reads as %q", text, strings.TrimSuffix(string(o.Format()), "\n"))
	}
	return o, nil
}
