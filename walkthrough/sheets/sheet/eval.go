package sheet

import (
	"maps"
	"math"
	"slices"
	"strconv"
)

// Values maps each cell's name to its value. A division by zero gives NaN,
// which Format writes as #DIV/0!, and a result too large for a float64 gives
// +Inf, which Format writes as #NUM!. Either passes through every operation
// that reads it, and where an operation reads both, its left operand's wins.
// Since NaN equals nothing, maps.Equal never reports Values holding a NaN equal.
type Values map[string]float64

// Format writes each cell and its value, one per line, in cell order: A1 6.
func (v Values) Format() []byte {
	var b []byte
	for _, name := range slices.SortedFunc(maps.Keys(v), compareNames) {
		b = append(b, name...)
		b = append(b, ' ')
		b = appendValue(b, v[name])
		b = append(b, '\n')
	}
	return b
}

func appendValue(b []byte, v float64) []byte {
	switch {
	case math.IsNaN(v):
		return append(b, "#DIV/0!"...)
	case math.IsInf(v, 0):
		return append(b, "#NUM!"...)
	case v == 0:
		return append(b, '0') // and never -0
	}
	return strconv.AppendFloat(b, v, 'f', -1, 64)
}

var (
	divZero  = math.NaN()
	overflow = math.Inf(1)
)

func isFault(v float64) bool { return math.IsNaN(v) || math.IsInf(v, 0) }

// Evaluate computes the value of every cell.
func (g *Graph) Evaluate() Values {
	v, _ := g.evaluate()
	return v
}

// evaluate computes the value of every cell, and returns as well the cells
// whose value is a division by zero of their own, in cell order.
func (g *Graph) evaluate() (Values, []*cell) {
	vals := make(Values, len(g.order))
	var divs []*cell
	for _, c := range g.order {
		if c.formula == nil {
			vals[c.name] = c.value
			continue
		}
		ev := evaluator{vals: vals}
		vals[c.name] = ev.eval(c.formula)
		if ev.divided {
			divs = append(divs, c)
		}
	}
	slices.SortFunc(divs, func(a, b *cell) int { return a.pos.compare(b.pos) })
	return vals, divs
}

type evaluator struct {
	vals    Values
	divided bool // some division in the formula had a zero divisor
}

func (ev *evaluator) eval(e expr) float64 {
	switch e := e.(type) {
	case number:
		return float64(e)
	case ref:
		return ev.vals[string(e)]
	case binary:
		// A fault stops the formula: nothing to its right is evaluated, so a
		// division past it never runs, and never warns.
		l := ev.eval(e.l)
		if isFault(l) {
			return l
		}
		r := ev.eval(e.r)
		if isFault(r) {
			return r
		}
		var v float64
		switch e.op {
		case '+':
			v = l + r
		case '-':
			v = l - r
		case '*':
			v = l * r
		case '/':
			if r == 0 {
				ev.divided = true
				return divZero
			}
			v = l / r
		}
		if math.IsInf(v, 0) {
			return overflow
		}
		return v
	}
	panic("sheet: unknown expression")
}
