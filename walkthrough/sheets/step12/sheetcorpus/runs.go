// walkthrough/sheets/step12/sheetcorpus/runs.go
package sheetcorpus

import (
	"maps"
	"sync"
)

// Runs counts each run of a lift, suite or transform the sheets declare,
// by its name, in this process: a lift's function, a suite's run, probe or
// classifier, and a transform's probe or jenny. The hypotheses read it to
// show what evaluating one runs.
var Runs runs

type runs struct {
	mu sync.Mutex
	n  map[string]int
}

// Add counts one run of the named lift, suite or transform.
func (r *runs) Add(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.n == nil {
		r.n = map[string]int{}
	}
	r.n[name]++
}

// Counts returns how many times each has run so far.
func (r *runs) Counts() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return maps.Clone(r.n)
}
