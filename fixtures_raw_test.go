package hypercase_test

import (
	"io"
	"io/fs"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/decl"
)

// The fixtures in the form of steps 3 to 5: suites over the raw input view,
// fs.FS, that name their Corpus, with no corpus type, given to Register
// bare.

// rawSteered is steered in that form, over testdata/script/.
func rawSteered(name, tree string) hypercase.Suite[fs.FS] {
	return hypercase.Suite[fs.FS]{
		Name:        name,
		Description: "Does what the case's " + tree + "/ tree says.",
		Corpus:      "script",
		Outputs:     steeredOutputs(tree),
		Run: func(t testing.TB, archiveName string, in fs.FS, stdout, stderr io.Writer) ([]hypercase.File, error) {
			return runSteered(t, archiveName, tree, in, stdout, stderr)
		},
	}
}

var (
	rawEcho  = rawSteered("echo", "do")
	rawShout = rawSteered("shout", "shout")
)

// rawProbe is withProbe in that form.
func rawProbe(s hypercase.Suite[fs.FS]) hypercase.Suite[fs.FS] {
	s.Probe = func(in fs.FS) (string, bool) { return steeredProbe(in, "do") }
	return s
}

// rawForms names, for each registration of the fixtures of steps 3 to 5,
// the registration that declares the same columns in the form of those
// steps.
var rawForms = map[string]string{
	"echo":        "raw-echo",
	"rejects":     "raw-rejects",
	"echo-shout":  "raw-echo-shout",
	"two-corpora": "raw-two-corpora",
	"probe":       "raw-probe",
	"narrow":      "raw-narrow",
	"pkg-a":       "raw-pkg-a",
	"pkg-b":       "raw-pkg-b",
}

var rawRegistrations = map[string]func(){
	"raw-echo": func() { hypercase.Register(project, rootPkg, rawEcho) },
	"raw-rejects": func() {
		hypercase.Register(project, rootPkg, circular, dangling, malformed, withRejects(rawEcho, circular, dangling))
	},
	"raw-echo-shout":  func() { hypercase.Register(project, rootPkg, rawEcho, rawShout) },
	"raw-two-corpora": func() { hypercase.Register(project, rootPkg, rawEcho, withCorpus(rawShout, "other")) },
	"raw-probe": func() {
		hypercase.Register(project, rootPkg, circular, withRejects(rawProbe(rawEcho), circular))
	},
	"raw-narrow": func() {
		hypercase.Register(project, rootPkg, withOutputs(rawEcho,
			decl.Output{Pattern: "values.txt", Description: "The values.", Inputs: steeredInputs("do")},
			decl.Output{Pattern: "?.txt", Description: "A file named with one letter.", Inputs: steeredInputs("do")},
			decl.Output{Pattern: "a.txt", Description: "The file a.", Inputs: steeredInputs("do")},
		))
	},
	"raw-pkg-a": func() { hypercase.Register(project, rootPkg, rawEcho) },
	"raw-pkg-b": func() { hypercase.Register(project, rawShout) },

	// A suite over the raw input view beside a corpus type of another
	// directory, and a suite over a corpus type given to Register bare.
	"raw-mixed": func() {
		hypercase.Register(project, rootPkg, rawEcho, otherType, own(steered[Other]("shout", "shout")))
	},
	"bare-echo": func() { hypercase.Register(project, rootPkg, scriptType, echo) },
	"bare-calc": func() { hypercase.Register(project, rootPkg, malformed, scriptType, calc) },

	// Refusals.
	"raw-no-corpus":  func() { hypercase.Register(project, withCorpus(rawEcho, "")) },
	"raw-bad-corpus": func() { hypercase.Register(project, withCorpus(rawEcho, "Script")) },
	"raw-checks":     func() { hypercase.Register(project, withCorpus(rawEcho, "checks")) },
	"raw-of-type":    func() { hypercase.Register(project, scriptType, rawEcho) },
	"raw-of-type-calls": func() {
		hypercase.Register(project, rawEcho)
		hypercase.Register(scriptType)
	},
	"raw-two-names": func() { hypercase.Register(project, rawEcho, own(rawEcho)) },
}
