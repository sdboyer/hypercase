package hypercase

import (
	"fmt"
	"path"

	"github.com/sdboyer/hypercase/decl"
)

// A statement is what an output declaration states its files' ranges come
// from, in its Inputs.
type statement int

const (
	// statesNone is a declaration that states nothing: a nil Inputs, or a
	// nil InputsFunc.
	statesNone statement = iota
	// statesFunc maps each file with an InputsFunc.
	statesFunc
	// statesAll states AllInputs.
	statesAll
	// statesPositions states FromPositions.
	statesPositions
	// statesOther is a value of the kind of AllInputs and FromPositions
	// that is neither, as arithmetic on them makes.
	statesOther
)

// stated is what an Inputs states.
func stated(in decl.Inputs) statement {
	switch f := in.(type) {
	case nil:
		return statesNone
	case decl.InputsFunc:
		if f == nil {
			return statesNone
		}
		return statesFunc
	}
	switch in {
	case decl.AllInputs:
		return statesAll
	case decl.FromPositions:
		return statesPositions
	}
	return statesOther
}

// checkInputs refuses what a column's output declarations state in Inputs
// that no declaration may: a value that is none of an InputsFunc,
// AllInputs and FromPositions; FromPositions on a pattern that isn't
// error.txt; and an InputsFunc on a pattern that matches error.txt, whose
// ranges come from Positions or AllInputs. A declaration that states
// nothing is left to the run, which fails on every such declaration at
// once.
func checkInputs(kind, name string, outputs []decl.Output) {
	for _, o := range outputs {
		matches, _ := path.Match(o.Pattern, errorFile)
		switch stated(o.Inputs) {
		case statesOther:
			refuse(fmt.Sprintf("the %s %s declares the output %q with an Inputs that is none of decl.InputsFunc, decl.AllInputs and decl.FromPositions. State one of them.", kind, name, o.Pattern))
		case statesPositions:
			if o.Pattern != errorFile {
				refuse(fmt.Sprintf("the %s %s declares the output %q with Inputs: decl.FromPositions, which only a declaration whose pattern is %s states: Positions finds where an error points, and only %s holds one. Give it a decl.InputsFunc or decl.AllInputs.", kind, name, o.Pattern, errorFile, errorFile))
			}
		case statesFunc:
			switch {
			case o.Pattern == errorFile:
				refuse(fmt.Sprintf("the %s %s declares the output %s with a decl.InputsFunc. Its ranges come from where the error points, through the project's Positions, or from the whole input view: state decl.FromPositions or decl.AllInputs.", kind, name, errorFile))
			case matches:
				refuse(fmt.Sprintf("the %s %s declares the output %q with a decl.InputsFunc, and the pattern matches %s, whose ranges come from the project's Positions or the whole input view, never from a function. State decl.AllInputs, or change the pattern so it doesn't match %s and declare %s on its own, with decl.FromPositions or decl.AllInputs.", kind, name, o.Pattern, errorFile, errorFile, errorFile))
			}
		}
	}
}

// checkProductInputs refuses what a bytes lift's product declarations
// state in Inputs that none may: a value that is none of an InputsFunc,
// AllInputs and FromPositions, and FromPositions, since no product file is
// an error.txt.
func checkProductInputs(l *lift) {
	for _, o := range l.product {
		switch stated(o.Inputs) {
		case statesOther:
			refuse(fmt.Sprintf("the lift %s declares the product file %q with an Inputs that is none of decl.InputsFunc, decl.AllInputs and decl.FromPositions. State a decl.InputsFunc or decl.AllInputs.", l.name, o.Pattern))
		case statesPositions:
			refuse(fmt.Sprintf("the lift %s declares the product file %q with Inputs: decl.FromPositions, and Positions finds where an error points, which a product file never holds. Give it a decl.InputsFunc or decl.AllInputs.", l.name, o.Pattern))
		}
	}
}
