package sheet

import (
	"slices"
	"strings"
)

// A Graph is a parsed sheet whose cells are ordered so that each comes after
// the cells it refers to.
type Graph struct {
	order []*cell
}

// Order returns the cells' names in the order Evaluate computes them.
func (g *Graph) Order() []string {
	names := make([]string, len(g.order))
	for i, c := range g.order {
		names[i] = c.name
	}
	return names
}

// Resolve orders the cells of p so that each comes after the cells it refers
// to. It fails with S202 on the first reference, in cell order, to a cell p
// doesn't have, and otherwise with S201 on the first cycle it finds. A cycle's
// message starts from its first cell in cell order: A1 -> B1 -> A1: cycle.
func Resolve(p *Parsed) (*Graph, error) {
	byName := make(map[string]*cell, len(p.cells))
	for _, c := range p.cells {
		byName[c.name] = c
	}
	for _, c := range p.cells {
		for _, r := range c.refs {
			if byName[r] == nil {
				return nil, newError(codeDangling, c.at(), "%s: %s doesn't exist", c.name, r)
			}
		}
	}

	const (
		unvisited = iota
		visiting
		done
	)
	state := make(map[*cell]int, len(p.cells))
	var stack, order []*cell
	var visit func(c *cell) error
	visit = func(c *cell) error {
		switch state[c] {
		case done:
			return nil
		case visiting:
			return cycleError(stack[slices.Index(stack, c):])
		}
		state[c] = visiting
		stack = append(stack, c)
		for _, r := range c.refs {
			if err := visit(byName[r]); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		state[c] = done
		order = append(order, c)
		return nil
	}
	for _, c := range p.cells {
		if err := visit(c); err != nil {
			return nil, err
		}
	}
	return &Graph{order: order}, nil
}

// cycleError reports the cycle through cells, each referring to the next and
// the last to the first.
func cycleError(cells []*cell) *Error {
	first := 0
	for i, c := range cells {
		if c.pos.compare(cells[first].pos) < 0 {
			first = i
		}
	}
	cells = append(slices.Clone(cells[first:]), cells[:first]...)
	names := make([]string, 0, len(cells)+1)
	at := make([]Position, 0, len(cells))
	for _, c := range cells {
		names = append(names, c.name)
		at = append(at, c.at()...)
	}
	names = append(names, cells[0].name)
	return newError(codeCycle, at, "%s: cycle", strings.Join(names, " -> "))
}
