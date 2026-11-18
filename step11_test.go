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

// TestInputsRequired: a run fails on each output declaration that states
// nothing in Inputs, naming it and what it may state, beside the check of
// descriptions: a suite's, a lift's, owned or used, and a product file's,
// owned or used; and, at once, on the declarations that state
// FromPositions in a project with no Positions. The cells still run: the
// update still writes their outputs.
func TestInputsRequired(t *testing.T) {
	t.Parallel()
	t.Run("suite", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "basic")
		want := []string{
			"the suite echo declares the output values.txt with no Inputs.\nState where its files come from: a decl.InputsFunc, which maps each file to the lines of the input view it comes from, or decl.AllInputs.",
			"the suite echo declares the output error.txt with no Inputs.\nState where its files come from: decl.FromPositions, which takes its ranges from where the error points, through the project's Positions, or decl.AllInputs.",
			"the suite echo declares the output e* with no Inputs, and the pattern matches error.txt.\nState decl.AllInputs, or change the pattern so it doesn't match error.txt and declare error.txt on its own, with decl.FromPositions or decl.AllInputs, and the rest with a decl.InputsFunc or decl.AllInputs.",
		}
		s.edit("testdata/script/twice.txtar", "echo's outputs, for the update to write", drop("out/echo/"))
		s.fails(s.update("inputs-none-suite"), want...)
		s.hasSections("testdata/script/twice.txtar", map[string]string{"out/echo/outcome": "accepted\n", "out/echo/values.txt": "A1 6\nB1 3\n"}, "out/echo/inputs")
		s.fails(s.normal("inputs-none-suite"), want...)
		s.done()
	})
	t.Run("lift", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "shared-ladder")
		want := []string{
			"the lift parse declares the output error.txt with no Inputs.\nState where its files come from: decl.FromPositions, which takes its ranges from where the error points, through the project's Positions, or decl.AllInputs.",
			"the lift parse declares the output stderr.txt with no Inputs.\nState where its files come from: a decl.InputsFunc, which maps each file to the lines of the input view it comes from, or decl.AllInputs.",
		}
		const x = "testdata/script/x.txtar"
		s.fails(s.update("inputs-none-lift-a"), want...)
		s.hasSections(x, map[string]string{"out/evaluate/values.txt": "evaluate 1\n"}, "out/parse/inputs", "out/evaluate/inputs")
		s.fails(s.run("b", "inputs-none-lift-b", true), want...)
		s.hasSections(x, map[string]string{"out/outline/values.txt": "outline 1\n"}, "out/outline/inputs")
		s.fails(s.normal("inputs-none-lift-a"), want...)
		s.fails(s.run("b", "inputs-none-lift-b", false), want...)
		s.done()
	})
	t.Run("product", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "chain-shared")
		want := []string{
			"the lift import declares the product file parse/* with no Inputs.\nState where its files come from: a decl.InputsFunc, which maps each file to the lines of the lift's input view it comes from, or decl.AllInputs.",
		}
		const prices = "testdata/export/prices.txtar"
		s.fails(s.update("inputs-none-product-a"), want...)
		s.hasSections(prices, map[string]string{"via/import/quote/A1": "3\n"}, "out/import/inputs", "out/parse/inputs")
		s.fails(s.run("b", "inputs-none-product-b", true), want...)
		s.hasSections(prices, map[string]string{"out/list/files.txt": "3 files\n"}, "out/list/inputs", "out/evaluate/inputs")
		s.fails(s.normal("inputs-none-product-a"), want...)
		s.fails(s.run("b", "inputs-none-product-b", false), want...)
		s.done()
	})
	t.Run("no-positions", func(t *testing.T) {
		t.Parallel()
		s := newSandbox(t, "ladder")
		want := "the project has no Positions, and the suite calc's error.txt, the suite evaluate's error.txt, the suite format's error.txt, the lift parse's error.txt and the lift resolve's error.txt state decl.FromPositions, which takes error.txt's ranges from it.\nGive the decl.Project its Positions, which finds where an error points, or state decl.AllInputs in place of decl.FromPositions."
		const fine = "testdata/script/fine.txtar"
		s.fails(s.update("inputs-no-positions"), want)
		s.hasSections(fine, map[string]string{"out/calc/values.txt": "calc 1\n"}, "out/calc/inputs")
		s.fails(s.normal("inputs-no-positions"), want)
		s.done()
	})
}
