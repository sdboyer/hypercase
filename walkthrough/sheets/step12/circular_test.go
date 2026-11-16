// walkthrough/sheets/step12/circular_test.go
package step12

import (
	"fmt"
	"io"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
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

// circularParsed is circular with its polarities from a predicate over
// parse's product, the parsed sheet, in place of what each case states:
// #20's question 2.
var circularParsed = hypothesis{
	name:        "circular-parsed",
	description: "circular, with each case's polarity from its parsed sheet: P when its formulas make a cycle, anti-P when they make none and refer to no missing cell.",
	property:    sheetcorpus.Circular,
	source:      parsed,
	claims:      circular.claims,
}

// parsed classifies a case for circular from what parse makes of its
// committed input: P when the parsed formulas make a cycle, neither when
// they make none and refer to a cell the sheet lacks, and anti-P
// otherwise. A case that parse rejects is unclassified. It runs parse's
// function, once per case, outside the framework: a predicate over a
// stage computes the stage.
var parsed = source{
	name: "parsed",
	runs: []string{"parse"},
	classify: func(k *kase) (casePolarity, error) {
		view, err := k.inputView()
		if err != nil {
			return "", err
		}
		p, err := sheetcorpus.Parse.Run(k.archive, sheetcorpus.Sheet{FS: view}, io.Discard)
		if err != nil {
			return unclassified, nil
		}
		// Parse accepted the sheet, so each file under sheet/ is a cell.
		entries, err := fs.ReadDir(view, "sheet")
		if err != nil {
			return "", err
		}
		cells := map[string]bool{}
		for _, e := range entries {
			cells[e.Name()] = true
		}
		// The references are read from the parsed formulas, as Format
		// prints them: A1 =B1 * 2.
		refs := map[string][]string{}
		for _, line := range strings.Split(string(sheet.Format(p)), "\n") {
			name, formula, ok := strings.Cut(line, " =")
			if !ok {
				continue
			}
			for _, tok := range strings.FieldsFunc(formula, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
				if cellName.MatchString(tok) {
					refs[name] = append(refs[name], tok)
				}
			}
		}
		if hasCycle(cells, refs) {
			return pos, nil
		}
		for _, rs := range refs {
			for _, r := range rs {
				if !cells[r] {
					return neither, nil
				}
			}
		}
		return anti, nil
	},
}

var cellName = regexp.MustCompile(`^[A-Z]{1,3}[1-9][0-9]*$`)

// hasCycle reports whether some cell refers back to itself, directly or
// through other cells. A reference to a cell the sheet lacks leads
// nowhere.
func hasCycle(cells map[string]bool, refs map[string][]string) bool {
	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	var visit func(c string) bool
	visit = func(c string) bool {
		switch state[c] {
		case visiting:
			return true
		case done:
			return false
		}
		state[c] = visiting
		for _, r := range refs[c] {
			if cells[r] && visit(r) {
				return true
			}
		}
		state[c] = done
		return false
	}
	for _, c := range slices.Sorted(maps.Keys(cells)) {
		if visit(c) {
			return true
		}
	}
	return false
}

// circularSources is no hypothesis: it records each case's polarity for
// circular from both sources, so a polarity that changes is a line of its
// findings that changes (#20's question 2), and a disagreement between the
// sources is a line that says so (question 3).
var circularSources = hypercase.Check{
	Name:        "circular-sources",
	Description: "Each case's polarity for circular, from what it states and from its parsed sheet, and whether the two disagree.",
	Run: func(cells []hypercase.Cell) ([]string, error) {
		return counted("circular-sources", parsed.runs, func() ([]string, int, error) {
			ks := cases(cells)
			var findings []string
			for _, k := range ks {
				s, err := stated.classify(k)
				if err != nil {
					return nil, 0, err
				}
				p, err := parsed.classify(k)
				if err != nil {
					return nil, 0, err
				}
				f := fmt.Sprintf("%s: stated %s, parsed %s", k.name, s, p)
				switch predicts := func(p casePolarity) bool { return p == pos || p == anti }; {
				case s == p:
				case !predicts(s) && !predicts(p):
					f += ", neither of which predicts anything"
				default:
					f += ", which disagree"
				}
				findings = append(findings, f)
			}
			return findings, len(ks), nil
		})
	},
}
