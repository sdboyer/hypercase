package hypercase_test

import (
	"testing"
)

// TestInputsRefusals: registration refuses what an output declaration's
// Inputs states that no declaration may, naming the declaration and what
// to state: FromPositions on a pattern that isn't error.txt, or on a
// product file; an InputsFunc on error.txt, or on a pattern that matches
// it; and a value of the constants' kind that is none of them. Each holds
// for a suite's declarations and a lift's, owned or used.
func TestInputsRefusals(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "chain")
	for _, tt := range []struct{ reg, want string }{
		{"inputs-positions-values", `panic: hypercase: the suite echo declares the output "values.txt" with Inputs: decl.FromPositions, which only a declaration whose pattern is error.txt states: Positions finds where an error points, and only error.txt holds one. Give it a decl.InputsFunc or decl.AllInputs.`},
		{"inputs-func-error", "panic: hypercase: the suite echo declares the output error.txt with a decl.InputsFunc. Its ranges come from where the error points, through the project's Positions, or from the whole input view: state decl.FromPositions or decl.AllInputs."},
		{"inputs-func-star", `panic: hypercase: the suite echo declares the output "*.txt" with a decl.InputsFunc, and the pattern matches error.txt, whose ranges come from the project's Positions or the whole input view, never from a function. State decl.AllInputs, or change the pattern so it doesn't match error.txt and declare error.txt on its own, with decl.FromPositions or decl.AllInputs.`},
		{"inputs-other", `panic: hypercase: the suite echo declares the output "values.txt" with an Inputs that is none of decl.InputsFunc, decl.AllInputs and decl.FromPositions. State one of them.`},
		{"inputs-lift-positions", `panic: hypercase: the lift parse declares the output "stderr.txt" with Inputs: decl.FromPositions, which only a declaration whose pattern is error.txt states: Positions finds where an error points, and only error.txt holds one. Give it a decl.InputsFunc or decl.AllInputs.`},
		{"inputs-lift-func", `panic: hypercase: the lift parse declares the output "*.txt" with a decl.InputsFunc, and the pattern matches error.txt, whose ranges come from the project's Positions or the whole input view, never from a function.`},
		{"inputs-product-positions", `panic: hypercase: the lift import declares the product file "parse/*" with Inputs: decl.FromPositions, and Positions finds where an error points, which a product file never holds. Give it a decl.InputsFunc or decl.AllInputs.`},
		{"inputs-product-other", `panic: hypercase: the lift import declares the product file "parse/*" with an Inputs that is none of decl.InputsFunc, decl.AllInputs and decl.FromPositions. State a decl.InputsFunc or decl.AllInputs.`},
	} {
		r := s.normal(tt.reg)
		s.fails(r, tt.want)
		s.lacks(r, "--- ")
	}
	s.done()
}
