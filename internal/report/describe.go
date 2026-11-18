package report

import (
	"cmp"
	"maps"
	"slices"

	"github.com/sdboyer/hypercase/internal/registry"
)

// A Described is something a registry or an archive describes, with its
// description: at B, or at A for what B lacks, which OnlyAtA records.
type Described struct {
	Name        string
	Description Text
	OnlyAtA     bool
}

// A Corpus describes one corpus, and what the registries declare over it.
// Its Name is its directory, and its Description its corpus type's.
type Corpus struct {
	Described
	Columns    []ColumnDescribed    // by name
	Transforms []TransformDescribed // its corpus type's, by name
	// Cases are the archives and hand-written variants only at A, by case,
	// which no row of the matrix at B describes.
	Cases []Described
}

// A ColumnDescribed is a column, with its output declarations and its
// classifier.
type ColumnDescribed struct {
	Described
	Outputs    []OutputDescribed // by pattern
	Classifier *Described
}

// An OutputDescribed is an output declaration, named by its pattern, with
// what its Inputs states at the revision it is described from: one of
// registry.InputsFunc, InputsPositions, InputsAll and InputsNone, or ""
// at a revision whose registries predate registry.InputsFormat. Positions
// records that the registry that claims its column there records that the
// project supplies Positions.
type OutputDescribed struct {
	Described
	Inputs    string
	Positions bool
}

// A TransformDescribed is a transform, with the properties it gives and
// keeps.
type TransformDescribed struct {
	Described
	Gives, Keeps []string
}

// describe returns the descriptions of each corpus and each property at B,
// and at A of what B lacks, each by name.
func describe(a, b *snapshot, links *linker) ([]Corpus, []Described) {
	// pick returns the snapshot to describe a thing from: B when it has
	// it, and A otherwise.
	pick := func(atB bool) (*snapshot, bool) {
		if atB {
			return b, false
		}
		return a, true
	}
	var corpora []Corpus
	inB := b.corpora()
	for _, dir := range union(a.corpora(), inB) {
		desc, ok := b.types[dir]
		if !ok {
			desc = a.types[dir]
		}
		c := Corpus{Described: Described{Name: dir, Description: links.text(desc), OnlyAtA: !inB[dir]}}

		for _, name := range union(a.columnsOver(dir), b.columnsOver(dir)) {
			colB, atB := b.column(dir, name)
			colA, atA := a.column(dir, name)
			col, onlyA := colB, false
			if !atB {
				col, onlyA = colA, true
			}
			cd := ColumnDescribed{Described: Described{Name: name, Description: links.text(col.description), OnlyAtA: onlyA}}
			output := func(o registry.Output, col *column, onlyA bool) OutputDescribed {
				return OutputDescribed{Described: Described{Name: o.Pattern, Description: links.text(o.Description), OnlyAtA: onlyA}, Inputs: o.Inputs, Positions: col.positions}
			}
			for _, o := range col.outputs {
				cd.Outputs = append(cd.Outputs, output(o, col, onlyA))
			}
			if cl := col.classifier; cl != nil {
				cd.Classifier = &Described{Name: cl.Name, Description: links.text(cl.Description), OnlyAtA: onlyA}
			}
			// What the column declares at A and not at B, B lacks: an
			// output declaration of another pattern, or a classifier. A
			// column in conflict at either revision has no declarations to
			// take from.
			if atA && atB && !colA.conflict && !colB.conflict {
				for _, o := range colA.outputs {
					if !slices.ContainsFunc(colB.outputs, func(p registry.Output) bool { return p.Pattern == o.Pattern }) {
						cd.Outputs = append(cd.Outputs, output(o, colA, true))
					}
				}
				if cl := colA.classifier; cl != nil && colB.classifier == nil {
					cd.Classifier = &Described{Name: cl.Name, Description: links.text(cl.Description), OnlyAtA: true}
				}
			}
			slices.SortFunc(cd.Outputs, func(x, y OutputDescribed) int { return cmp.Compare(x.Name, y.Name) })
			c.Columns = append(c.Columns, cd)
		}

		for _, name := range union(a.transforms[dir], b.transforms[dir]) {
			_, atB := b.transforms[dir][name]
			s, onlyA := pick(atB)
			tr := s.transforms[dir][name]
			c.Transforms = append(c.Transforms, TransformDescribed{
				Described: Described{Name: name, Description: links.text(tr.Description), OnlyAtA: onlyA},
				Gives:     tr.Gives, Keeps: tr.Keeps,
			})
		}

		var gone []caseKey
		for k := range a.cases {
			if _, ok := b.cases[k]; !ok && k.corpus == dir && k.kind != transform {
				gone = append(gone, k)
			}
		}
		slices.SortFunc(gone, caseKey.compare)
		for _, k := range gone {
			c.Cases = append(c.Cases, Described{Name: k.String(), Description: links.text(a.cases[k].description), OnlyAtA: true})
		}
		corpora = append(corpora, c)
	}

	var props []Described
	for _, name := range union(a.props, b.props) {
		_, atB := b.props[name]
		s, onlyA := pick(atB)
		props = append(props, Described{Name: name, Description: links.text(s.props[name]), OnlyAtA: onlyA})
	}
	return corpora, props
}

// corpora returns the directories of the corpora the registries declare:
// each corpus type's, and each a column runs over.
func (s *snapshot) corpora() map[string]bool {
	out := map[string]bool{}
	for dir := range s.types {
		out[dir] = true
	}
	for _, c := range s.columns {
		for dir := range c.corpora {
			out[dir] = true
		}
	}
	return out
}

// columnsOver returns the columns the registries claim over a corpus.
func (s *snapshot) columnsOver(corpus string) map[string]bool {
	out := map[string]bool{}
	for name, c := range s.columns {
		if c.corpora[corpus] {
			out[name] = true
		}
	}
	return out
}

// union returns the keys of two maps, sorted.
func union[V1, V2 any](x map[string]V1, y map[string]V2) []string {
	keys := map[string]bool{}
	for k := range x {
		keys[k] = true
	}
	for k := range y {
		keys[k] = true
	}
	return slices.Sorted(maps.Keys(keys))
}
