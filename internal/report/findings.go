package report

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/internal/diff"
)

// A FindingsChange is what changed in one corpus check's findings between
// the revisions.
type FindingsChange struct {
	// Package is the directory of the package that registers the check,
	// from the top of the repository, and Check is the check's name.
	Package, Check string
	// Description is the check's, as the package's registry lists it at B,
	// or at A where B's doesn't. It is empty where neither does, as a
	// registry from before registry.ChecksFormat lists none.
	Description Text
	// Disappeared are the findings only at A, and Appeared those only at
	// B, each in its findings file's order, and each as a diff shows a
	// line.
	Disappeared, Appeared []string
}

// A checkKey names a corpus check: the directory of the package that
// registers it, from the top of the repository, and its name, which is
// unique per package.
type checkKey struct {
	pkg, name string
}

func (k checkKey) compare(o checkKey) int {
	if c := strings.Compare(k.pkg, o.pkg); c != 0 {
		return c
	}
	return strings.Compare(k.name, o.name)
}

// readFindings reads the findings files of each package whose registry
// the snapshot reads, in its testdata/checks/. From registry.ChecksFormat
// on, a registry lists its package's checks, each with its description,
// and the report reads each listed check's findings, and lists as a
// problem a listed check with no findings file, one with something other
// than a regular file in its findings file's place, and each other file
// there, which is no listed check's findings. Before it, the report reads
// each file whose name is a check's under the naming rule, then .txt, as
// strip finds them, with no description and neither problem. A findings
// file that isn't as an update writes it is a problem, and the report
// shows no change in its check's findings at this revision. A package's
// problems are listed by path.
func (s *snapshot) readFindings(tree fs.FS, regs []found) error {
	for _, f := range regs {
		pkg := path.Dir(path.Dir(f.path))
		dir := path.Join(pkg, "testdata", archive.ChecksDir)
		entries, err := fs.ReadDir(tree, dir)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("reading %s at %s: %w", dir, s.rev.Name, err)
		}
		var problems []Problem
		problem := func(where, msg string) {
			problems = append(problems, Problem{At: s.rev.Name, Where: where, Msg: msg})
		}
		listed := f.reg.Checks != nil
		owns := map[string]bool{}
		for _, c := range f.reg.Checks {
			owns[c.Name] = true
			s.checks[checkKey{pkg: pkg, name: c.Name}] = c.Description
		}
		files := map[string]bool{}     // by check name
		irregular := map[string]bool{} // by check name, where no regular file is in its findings file's place
		for _, e := range entries {
			name, txt := strings.CutSuffix(e.Name(), ".txt")
			ok := txt && e.Type().IsRegular()
			switch {
			case listed && ok && owns[name], !listed && ok && archive.ValidName(name):
				files[name] = true
			case listed && txt && owns[name]:
				irregular[name] = true
				problem(path.Join(dir, e.Name()), fmt.Sprintf("%s lists the corpus check %s, and this isn't a regular file, as the findings file an update writes for it is. So the report shows no change in its findings here.", f.path, name))
			case listed:
				problem(path.Join(dir, e.Name()), fmt.Sprintf("the file is the findings of no corpus check %s lists, so the report leaves it out.", f.path))
			}
		}
		for _, c := range f.reg.Checks {
			if files[c.Name] {
				continue
			}
			s.unread[checkKey{pkg: pkg, name: c.Name}] = true
			if !irregular[c.Name] {
				problem(path.Join(dir, c.Name+".txt"), fmt.Sprintf("%s lists the corpus check %s, and the package has no findings file for it, which an update writes for each check it lists. So the report shows no change in its findings here.", f.path, c.Name))
			}
		}
		for _, name := range slices.Sorted(maps.Keys(files)) {
			k := checkKey{pkg: pkg, name: name}
			p := path.Join(dir, name+".txt")
			data, err := fs.ReadFile(tree, p)
			if err != nil {
				return fmt.Errorf("reading %s at %s: %w", p, s.rev.Name, err)
			}
			findings, why := parseFindings(data)
			if why != "" {
				s.unread[k] = true
				problem(p, fmt.Sprintf("the findings file isn't as an update writes it: %s. So the report shows no change in the check's findings here.", why))
				continue
			}
			s.findings[k] = findings
		}
		slices.SortStableFunc(problems, func(x, y Problem) int { return cmp.Compare(x.Where, y.Where) })
		s.problems = append(s.problems, problems...)
	}
	return nil
}

// parseFindings reads a findings file as an update writes it: one finding
// per line, none empty, sorted byte by byte, each once, every line ending
// with a newline, and no findings as an empty file. Otherwise it says why
// the file isn't.
func parseFindings(data []byte) (findings []string, why string) {
	if len(data) == 0 {
		return nil, ""
	}
	text, ok := strings.CutSuffix(string(data), "\n")
	if !ok {
		return nil, "its last line has no newline"
	}
	findings = strings.Split(text, "\n")
	for i, f := range findings {
		switch {
		case f == "":
			return nil, fmt.Sprintf("line %d is empty", i+1)
		case i > 0 && f == findings[i-1]:
			return nil, fmt.Sprintf("line %d repeats line %d", i+1, i)
		case i > 0 && f < findings[i-1]:
			return nil, fmt.Sprintf("line %d sorts before line %d", i+1, i)
		}
	}
	return findings, ""
}

// findingsChanges returns what changed in each corpus check's findings,
// by package, then check: the findings only at A, and only at B, as sets.
// A check with no findings file at a revision has no findings there,
// except where its registry lists it, or where its file isn't as an
// update writes it, each a problem, and where the report shows no change
// in it.
func findingsChanges(a, b *snapshot, links *linker) []FindingsChange {
	keys := map[checkKey]bool{}
	for _, s := range []*snapshot{a, b} {
		for k := range s.findings {
			keys[k] = true
		}
		for k := range s.checks {
			keys[k] = true
		}
	}
	var out []FindingsChange
	for _, k := range slices.SortedFunc(maps.Keys(keys), checkKey.compare) {
		if a.unread[k] || b.unread[k] {
			continue
		}
		fc := FindingsChange{Package: k.pkg, Check: k.name, Disappeared: displayed(only(a.findings[k], b.findings[k])), Appeared: displayed(only(b.findings[k], a.findings[k]))}
		if fc.Appeared == nil && fc.Disappeared == nil {
			continue
		}
		desc, ok := b.checks[k]
		if !ok {
			desc = a.checks[k]
		}
		fc.Description = links.text(desc)
		out = append(out, fc)
	}
	return out
}

// only returns the lines of one list that the other lacks, in order.
func only(lines, other []string) []string {
	in := map[string]bool{}
	for _, l := range other {
		in[l] = true
	}
	var out []string
	for _, l := range lines {
		if !in[l] {
			out = append(out, l)
		}
	}
	return out
}

// displayed returns lines as a diff shows them.
func displayed(lines []string) []string {
	var out []string
	for _, l := range lines {
		out = append(out, diff.Display(l))
	}
	return out
}
