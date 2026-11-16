// walkthrough/sheets/step12/circular_test.go
package step12

import (
	"slices"

	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step12/sheetcorpus"
)

// circular is #20's first hypothesis. resolve is P: a case with a cycle is
// rejected, and one without accepted. evaluate is P: a case with a cycle is
// blocked at resolve. format is neutral: over a pair, a sheet with no cycle
// and its variant with one, both cases have the same outcome.
var circular = hypothesis{
	name:        "circular",
	description: "resolve rejects a sheet with a cycle and accepts one without, evaluate is blocked at resolve on a sheet with a cycle, and format's outcome doesn't depend on whether a sheet has one. A case's polarity is what it states.",
	property:    sheetcorpus.Circular,
	source:      stated,
	claims: []claim{
		{column: "resolve", polarity: colP, match: "rejected", mismatch: "accepted"},
		{column: "evaluate", polarity: colP, match: "blocked resolve"},
		{column: "format", polarity: colNeutral, pair: []pairPrediction{sameOutcome}},
	},
}

// circularRelabelled is circular with its P columns written as anti-P,
// each with its predictions swapped: #20's question 5 asks whether that
// claims anything different on the outcome axis.
var circularRelabelled = hypothesis{
	name:        "circular-relabelled",
	description: "circular, with resolve and evaluate claimed anti-P and their predictions swapped.",
	property:    sheetcorpus.Circular,
	source:      stated,
	claims: []claim{
		{column: "resolve", polarity: colAnti, match: "accepted", mismatch: "rejected"},
		{column: "evaluate", polarity: colAnti, mismatch: "blocked resolve"},
		{column: "format", polarity: colNeutral, pair: []pairPrediction{sameOutcome}},
	},
}

// stated classifies a case for circular from the properties it states,
// read as a closed world: a case states every property it has. It is P
// when it states circular, and anti-P when it states none of the three. A
// case that states malformed never reaches resolve, and one that states
// dangling and not circular is rejected at resolve for another reason, so
// either is neither.
var stated = source{
	name: "stated",
	classify: func(k *kase) (casePolarity, error) {
		has := func(p decl.Property) bool { return slices.Contains(k.properties, p.Name) }
		switch {
		case has(sheetcorpus.Malformed):
			return neither, nil
		case has(sheetcorpus.Circular):
			return pos, nil
		case has(sheetcorpus.Dangling):
			return neither, nil
		}
		return anti, nil
	},
}
