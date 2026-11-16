package sheet

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// A formula is an expression over numbers and cell references, with the
// operators + - * / and parentheses. * and / bind tighter than + and -, and
// operators of one precedence group to the left. A number is digits, with an
// optional fraction: 3, 2.5. There is no unary minus.

type tokenKind int

const (
	tNumber tokenKind = iota
	tRef
	tOp
	tOpen
	tClose
	tWord // any other run of characters
)

type token struct {
	kind tokenKind
	text string
	off  int // the byte offset of text in the lexed string
}

func isDelim(c byte) bool {
	switch c {
	case ' ', '\t', '+', '-', '*', '/', '(', ')':
		return true
	}
	return false
}

// lex splits s into tokens. It never fails: what is neither a number, a
// reference, an operator nor a parenthesis is a word.
func lex(s string) []token {
	var toks []token
	for i := 0; i < len(s); {
		c := s[i]
		switch c {
		case ' ', '\t':
			i++
			continue
		case '+', '-', '*', '/':
			toks = append(toks, token{tOp, s[i : i+1], i})
			i++
			continue
		case '(':
			toks = append(toks, token{tOpen, "(", i})
			i++
			continue
		case ')':
			toks = append(toks, token{tClose, ")", i})
			i++
			continue
		}
		j := i
		for j < len(s) && !isDelim(s[j]) {
			j++
		}
		w := s[i:j]
		kind := tWord
		if isNumber(w) {
			kind = tNumber
		} else if _, ok := parseName(w); ok {
			kind = tRef
		}
		toks = append(toks, token{kind, w, i})
		i = j
	}
	return toks
}

func isNumber(w string) bool {
	whole, frac, dot := strings.Cut(w, ".")
	return allDigits(whole) && (!dot || allDigits(frac))
}

func allDigits(s string) bool {
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

// parseNumber reads a number token. One too large for a float64 is an error;
// one too small rounds to zero.
func parseNumber(text string) (float64, error) {
	v, _ := strconv.ParseFloat(text, 64)
	if math.IsInf(v, 0) {
		return 0, fmt.Errorf("number out of range: %q", text)
	}
	return v, nil
}

// An expr is a parsed formula.
type expr interface{ prec() int }

type number float64

type ref string

type binary struct {
	op   byte
	l, r expr
}

func (number) prec() int { return 3 }
func (ref) prec() int    { return 3 }

func (b binary) prec() int {
	if b.op == '+' || b.op == '-' {
		return 1
	}
	return 2
}

// appendExpr prints e in canonical form: one space around each operator,
// parentheses only where the grouping needs them, and each number in its
// shortest decimal form.
func appendExpr(b []byte, e expr) []byte {
	switch e := e.(type) {
	case number:
		return strconv.AppendFloat(b, float64(e), 'f', -1, 64)
	case ref:
		return append(b, e...)
	case binary:
		b = appendOperand(b, e.l, e.l.prec() < e.prec())
		b = append(b, ' ', e.op, ' ')
		// Operators group to the left, so a right operand of the same
		// precedence keeps its parentheses: A1 - (B1 - C1).
		return appendOperand(b, e.r, e.r.prec() <= e.prec())
	}
	panic(fmt.Sprintf("sheet: unknown expression %T", e))
}

func appendOperand(b []byte, e expr, paren bool) []byte {
	if !paren {
		return appendExpr(b, e)
	}
	b = append(b, '(')
	b = appendExpr(b, e)
	return append(b, ')')
}

// refsOf returns the cells e refers to, each once, in the order they first
// appear.
func refsOf(e expr) []string {
	var refs []string
	var walk func(expr)
	walk = func(e expr) {
		switch e := e.(type) {
		case ref:
			for _, r := range refs {
				if r == string(e) {
					return
				}
			}
			refs = append(refs, string(e))
		case binary:
			walk(e.l)
			walk(e.r)
		}
	}
	walk(e)
	return refs
}

var errEnd = errors.New("unexpected end of formula")

// parseFormula parses the text after a formula's =.
func parseFormula(s string) (expr, error) {
	p := &parser{toks: lex(s)}
	e, err := p.sum()
	if err != nil {
		return nil, err
	}
	if p.i < len(p.toks) {
		return nil, p.unexpected()
	}
	return e, nil
}

type parser struct {
	toks []token
	i    int
}

func (p *parser) op(ops string) (byte, bool) {
	if p.i < len(p.toks) && p.toks[p.i].kind == tOp && strings.Contains(ops, p.toks[p.i].text) {
		op := p.toks[p.i].text[0]
		p.i++
		return op, true
	}
	return 0, false
}

func (p *parser) sum() (expr, error) {
	l, err := p.product()
	for err == nil {
		op, ok := p.op("+-")
		if !ok {
			return l, nil
		}
		var r expr
		r, err = p.product()
		l = binary{op, l, r}
	}
	return nil, err
}

func (p *parser) product() (expr, error) {
	l, err := p.operand()
	for err == nil {
		op, ok := p.op("*/")
		if !ok {
			return l, nil
		}
		var r expr
		r, err = p.operand()
		l = binary{op, l, r}
	}
	return nil, err
}

func (p *parser) operand() (expr, error) {
	if p.i == len(p.toks) {
		return nil, errEnd
	}
	t := p.toks[p.i]
	switch t.kind {
	case tNumber:
		p.i++
		v, err := parseNumber(t.text)
		return number(v), err
	case tRef:
		p.i++
		return ref(t.text), nil
	case tOpen:
		p.i++
		e, err := p.sum()
		if err != nil {
			return nil, err
		}
		if p.i == len(p.toks) || p.toks[p.i].kind != tClose {
			return nil, p.unexpected()
		}
		p.i++
		return e, nil
	}
	return nil, p.unexpected()
}

func (p *parser) unexpected() error {
	if p.i == len(p.toks) {
		return errEnd
	}
	return fmt.Errorf("unexpected %q", p.toks[p.i].text)
}
