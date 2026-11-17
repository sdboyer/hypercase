package hypercase_test

import (
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/jenny"
)

// The topology grids are TestClosings' grids made from every shape a chain
// of lifts takes, where each hand-picked grid takes one. A topology is a
// chain of one to maxChain lifts from the corpus type Top0, of
// testdata/top0/, each a bytes lift (B) or a stage (N), then the suite list
// over the type the chain ends in, and the package that owns each of them:
// the module's root package, a, or ./b, with a owning one at least. A
// package uses the lifts on its columns' paths that it doesn't own.
//
// The lift at position k is lk. It rejects the property pk, and follows its
// input view's lk/ tree: a bytes lift as steerImport does, making its
// lk/out/ tree its product, and a stage as steerLift does, passing its input
// view on. So the base's data, data/A1 and data/B1, sits below the out/ tree
// of each bytes lift in turn, and list lists the view the chain ends in.
//
// A topology's grid crosses each way the hand-picked grids make a lift fail,
// for each lift on the chain, with each edit of what the archive commits
// that they make. A lift fails by rejecting the case, which states the
// property it rejects or doesn't, by its jenny's fault, for a bytes lift, or
// by its panic. The archive commits a bytes lift's product with a file
// changed, a file short or a file extra, or with a lift's tree saying it
// fails, in each way, for each lift that reads the product, a lift's outcome
// as rejected, or list's files stale or with a stray. Every scenario is a
// hand-written variant of one archive, which each run reads at once.

// maxChain is the longest chain a topology has. Three is the shortest
// chain in which the lifts a closing tells apart on a cell's path, the one
// that fails, the last bytes lift and the last one the cell's package
// doesn't own, stand in every order to one another. Chains of four lifts
// add 138,849 scenarios to the 21,548 of chains up to three.
const maxChain = 3

// The types a topology's lifts take and make: Top0, the corpus type, and at
// position k, TopkB, the product of a bytes lift, a corpus type of
// testdata/topk/, which holds no archive, or *TopkN, a stage's.
type (
	Top0  struct{ fs.FS }
	Top1B struct{ fs.FS }
	Top2B struct{ fs.FS }
	Top3B struct{ fs.FS }
	Top1N struct{ view fs.FS }
	Top2N struct{ view fs.FS }
	Top3N struct{ view fs.FS }
)

// A topologyValue is a value a topology's lift or list takes, which gives
// the input view it comes from.
type topologyValue interface{ topologyView() fs.FS }

func (v Top0) topologyView() fs.FS   { return v.FS }
func (v Top1B) topologyView() fs.FS  { return v.FS }
func (v Top2B) topologyView() fs.FS  { return v.FS }
func (v Top3B) topologyView() fs.FS  { return v.FS }
func (v *Top1N) topologyView() fs.FS { return v.view }
func (v *Top2N) topologyView() fs.FS { return v.view }
func (v *Top3N) topologyView() fs.FS { return v.view }

// A topology is a chain's kinds, a letter for each lift, B or N, and its
// owners, a letter for each lift and then list's, a or b.
type topology struct{ kinds, owners string }

// topologies are every chain of one to maxChain lifts, crossed with every
// assignment of its lifts and list to a and b in which a owns one at least.
func topologies() []topology {
	chains := []string{""}
	var all []string
	for range maxChain {
		var next []string
		for _, c := range chains {
			next = append(next, c+"B", c+"N")
		}
		chains = next
		all = append(all, chains...)
	}
	var out []topology
	for _, kinds := range all {
		for mask := range 1 << (len(kinds) + 1) {
			var owners strings.Builder
			for i := range len(kinds) + 1 {
				owners.WriteByte("ab"[mask>>i&1])
			}
			if strings.Contains(owners.String(), "a") {
				out = append(out, topology{kinds, owners.String()})
			}
		}
	}
	return out
}

func (tp topology) name() string { return "topology-" + tp.kinds + "-" + tp.owners }

// registration names the registration of the package pkg, a or b, which
// topologyRegistration reads.
func (tp topology) registration(pkg byte) string { return tp.name() + "-" + string(pkg) }

// topologyRegistration is the registration a name gives, as a topology's
// registration names it.
func topologyRegistration(name string) (func(), bool) {
	rest, ok := strings.CutPrefix(name, "topology-")
	if !ok {
		return nil, false
	}
	parts := strings.Split(rest, "-")
	if len(parts) != 3 || len(parts[2]) != 1 {
		panic("selftest: no topology registration named " + name)
	}
	tp := topology{parts[0], parts[1]}
	return func() { tp.register(parts[2][0]) }, true
}

// register registers the package pkg, a or b, of the topology: the corpus
// types and properties of its chain, the columns it owns, and the lifts it
// uses, those on its columns' paths that it doesn't own.
func (tp topology) register(pkg byte) {
	items := []any{project, decl.Type[Top0]{Dir: "top0", Description: "Trees of instructions, under in/<lift>/, that a chain of lifts follows, and the data, under in/data/ or below a bytes lift's tree."}}
	if pkg == 'a' {
		items = append(items, rootPkg)
	}
	for k := 1; k <= len(tp.kinds); k++ {
		items = append(items, topologyProperty(k))
		if tp.kinds[k-1] == 'B' {
			items = append(items, topologyType(k))
		}
	}
	last := strings.LastIndexByte(tp.owners, pkg)
	var owned, used []any
	for i := range tp.owners {
		switch col := tp.column(i); {
		case tp.owners[i] == pkg:
			owned = append(owned, col)
		case i < last:
			used = append(used, col)
		}
	}
	items = append(items, own(owned...))
	if len(used) > 0 {
		items = append(items, hypercase.Use(used...))
	}
	hypercase.Register(items...)
}

func topologyProperty(k int) decl.Property {
	return decl.Property{Name: fmt.Sprintf("p%d", k), Description: fmt.Sprintf("The case is one the lift l%d rejects.", k)}
}

func topologyType(k int) any {
	desc := fmt.Sprintf("The products of the bytes lift l%d.", k)
	switch k {
	case 1:
		return decl.Type[Top1B]{Dir: "top1", Description: desc}
	case 2:
		return decl.Type[Top2B]{Dir: "top2", Description: desc}
	}
	return decl.Type[Top3B]{Dir: "top3", Description: desc}
}

// column is the topology's column at index i: the lift at position i+1, or
// list, past the chain.
func (tp topology) column(i int) any {
	if i == len(tp.kinds) {
		return topologyList(tp.kinds)
	}
	k := i + 1
	switch {
	case k == 1:
		return topologyLift[Top0](tp.kinds, k)
	case k == 2 && tp.kinds[0] == 'B':
		return topologyLift[Top1B](tp.kinds, k)
	case k == 2:
		return topologyLift[*Top1N](tp.kinds, k)
	case tp.kinds[1] == 'B':
		return topologyLift[Top2B](tp.kinds, k)
	}
	return topologyLift[*Top2N](tp.kinds, k)
}

// topologyLift is the lift at position k of a chain, on In, the type the
// lift before it makes.
func topologyLift[In topologyValue](kinds string, k int) any {
	switch b := kinds[k-1] == 'B'; {
	case k == 1 && b:
		return topologyBytes[In, Top1B](kinds, k)
	case k == 1:
		return topologyStage[In](k, func(v fs.FS) *Top1N { return &Top1N{v} })
	case k == 2 && b:
		return topologyBytes[In, Top2B](kinds, k)
	case k == 2:
		return topologyStage[In](k, func(v fs.FS) *Top2N { return &Top2N{v} })
	case b:
		return topologyBytes[In, Top3B](kinds, k)
	}
	return topologyStage[In](k, func(v fs.FS) *Top3N { return &Top3N{v} })
}

// topologyTree is where the tree of the lift at position j sits, or the
// data, for j past the chain, in the input view the lift at position from
// makes, or in the case's, for from 0: below the out/ tree of each bytes
// lift between them.
func topologyTree(kinds string, from, j int) string {
	var b strings.Builder
	for m := from + 1; m < j; m++ {
		if kinds[m-1] == 'B' {
			fmt.Fprintf(&b, "l%d/out/", m)
		}
	}
	if j > len(kinds) {
		return b.String() + "data/"
	}
	return b.String() + fmt.Sprintf("l%d/", j)
}

func topologyBytes[In topologyValue, Out ~struct{ fs.FS }](kinds string, k int) decl.Lift[In, Out] {
	name := fmt.Sprintf("l%d", k)
	var product []decl.Output
	for j := k + 1; j <= len(kinds); j++ {
		product = append(product, decl.Output{Pattern: topologyTree(kinds, k, j) + "*", Description: fmt.Sprintf("What l%d does, as its tree says.", j)})
	}
	product = append(product, decl.Output{Pattern: topologyTree(kinds, k, len(kinds)+1) + "*", Description: "The data list lists."})
	return decl.Lift[In, Out]{
		Name:        name,
		Description: fmt.Sprintf("Makes its input view's %s/out/ tree its product, as its %s/ tree says.", name, name),
		Outputs:     []decl.Output{{Pattern: "error.txt", Description: "Why the lift rejects the case."}},
		Product:     product,
		Rejects:     []decl.Property{topologyProperty(k)},
		Jenny: jenny.For[In]().New(name, func(in In) (jenny.Files, error) {
			return steerImport(name, in.topologyView())
		}),
	}
}

func topologyStage[In topologyValue, Out any](k int, mk func(fs.FS) Out) decl.Lift[In, Out] {
	name := fmt.Sprintf("l%d", k)
	return decl.Lift[In, Out]{
		Name:        name,
		Description: fmt.Sprintf("Passes its input view on, as its %s/ tree says.", name),
		Outputs:     liftOutputs,
		Rejects:     []decl.Property{topologyProperty(k)},
		Run: func(archiveName string, in In, stderr io.Writer) (Out, error) {
			if err := steerLift(name, archiveName, in.topologyView(), stderr); err != nil {
				var none Out
				return none, err
			}
			return mk(in.topologyView()), nil
		},
	}
}

// topologyList is list over the type a chain ends in.
func topologyList(kinds string) any {
	switch b := kinds[len(kinds)-1] == 'B'; {
	case len(kinds) == 1 && b:
		return topologySuite[Top1B]()
	case len(kinds) == 1:
		return topologySuite[*Top1N]()
	case len(kinds) == 2 && b:
		return topologySuite[Top2B]()
	case len(kinds) == 2:
		return topologySuite[*Top2N]()
	case b:
		return topologySuite[Top3B]()
	}
	return topologySuite[*Top3N]()
}

func topologySuite[T topologyValue]() hypercase.Suite[T] {
	return hypercase.Suite[T]{
		Name:        "list",
		Description: "Lists the files of the input view the chain ends in, with their bytes.",
		Outputs:     []decl.Output{{Pattern: "files.txt", Description: "How many files the view holds, then each one's path and bytes, quoted."}},
		Run: func(t testing.TB, _ string, in T, _, _ io.Writer) ([]hypercase.File, error) {
			view := in.topologyView()
			paths, err := archive.Paths(view)
			if err != nil {
				return nil, err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "%d files\n", len(paths))
			for _, p := range paths {
				data, err := fs.ReadFile(view, p)
				if err != nil {
					return nil, err
				}
				fmt.Fprintf(&b, "%s %q\n", p, data)
			}
			return []hypercase.File{{Path: "files.txt", Data: []byte(b.String())}}, nil
		},
	}
}

// topologyGrids are the topology grids, one for each topology.
func topologyGrids() []closingGrid {
	var gs []closingGrid
	for _, tp := range topologies() {
		gs = append(gs, tp.grid())
	}
	return gs
}

// A topologyEdit is one side of a topology's scenario: an edit of the
// variant whose sections start with a prefix, which says what it does.
type topologyEdit struct {
	name, does string
	fn         func(prefix string) func(a *archive.Archive)
}

// grid is the topology's grid. Each scenario is a hand-written variant of
// the base, which the first update writes as it writes the base, and which
// the scenario then edits: a way one lift fails, under the variant's in/,
// crossed with an edit of what the archive commits for the variant.
func (tp topology) grid() closingGrid {
	n := len(tp.kinds)
	g := closingGrid{name: tp.name(), archive: "testdata/top0/base.txtar", packed: true, unowned: map[string]string{}}
	two := strings.Contains(tp.owners, "b")
	g.pkgs = []childRun{{pkg: "", registration: tp.registration('a')}}
	if two {
		g.pkgs = append(g.pkgs, childRun{pkg: "b", registration: tp.registration('b')})
	}
	// Each column's cell, in the package that owns it, past the last bytes
	// lift on its path that the package doesn't own.
	colName := func(i int) string {
		if i == n {
			return "list"
		}
		return fmt.Sprintf("l%d", i+1)
	}
	for i := range tp.owners {
		for j := i - 1; j >= 0; j-- {
			if tp.kinds[j] == 'B' && tp.owners[j] != tp.owners[i] {
				pkg := map[byte]string{'a': ".", 'b': "b"}[tp.owners[i]]
				g.unowned[pkg+" "+colName(i)] = colName(j)
				break
			}
		}
	}
	none := func(string) func(*archive.Archive) { return func(*archive.Archive) {} }
	setAt := func(name, data string) func(string) func(*archive.Archive) {
		return func(prefix string) func(*archive.Archive) { return set(prefix+name, data) }
	}
	fails := []topologyEdit{{"none", "no lift fails", none}}
	commits := []topologyEdit{{"none", "the archive commits what the first update wrote", none}}
	data := func(k int) string { return fmt.Sprintf("via/l%d/%s", k, topologyTree(tp.kinds, k, n+1)) }
	for k := 1; k <= n; k++ {
		l := fmt.Sprintf("l%d", k)
		tree := "in/" + topologyTree(tp.kinds, 0, k)
		stated := func(prefix string) func(*archive.Archive) {
			return seq(set(prefix+"properties", fmt.Sprintf("p%d\n", k)), set(prefix+tree+"error", l+" rejects the case\n"))
		}
		fails = append(fails,
			topologyEdit{l + "-rejects-stated", fmt.Sprintf("%s rejects the case, which states p%d", l, k), stated},
			topologyEdit{l + "-rejects", l + " rejects the case, which states no property", setAt(tree+"error", l+" rejects the case\n")})
		if tp.kinds[k-1] == 'B' {
			short := func(prefix string) func(*archive.Archive) { return drop(prefix + data(k) + "A1") }
			fails = append(fails, topologyEdit{l + "-faults", l + "'s jenny fails", setAt(tree+"fail", l+"'s jenny fails\n")})
			commits = append(commits,
				topologyEdit{l + "-product-changed", "the archive commits " + l + "'s product with a file changed", setAt(data(k)+"A1", "9\n")},
				topologyEdit{l + "-product-short", "the archive commits " + l + "'s product short of a file", short},
				topologyEdit{l + "-product-extra", "the archive commits " + l + "'s product with a file it doesn't make", setAt(data(k)+"Z9", "9\n")})
			// A lift that reads the product, up to the next bytes lift, fails on
			// what it commits, in each way, where it doesn't on what lk makes.
			for j := k + 1; j <= n; j++ {
				lj, at := fmt.Sprintf("l%d", j), fmt.Sprintf("via/l%d/%s", k, topologyTree(tp.kinds, k, j))
				was := "the archive commits " + l + "'s product with " + lj + "'s tree saying "
				commits = append(commits, topologyEdit{l + "-product-makes-" + lj + "-reject", was + "it rejects the case", setAt(at+"error", lj+" rejects the case\n")})
				if tp.kinds[j-1] == 'B' {
					commits = append(commits, topologyEdit{l + "-product-makes-" + lj + "-fault", was + "its jenny fails", setAt(at+"fail", lj+"'s jenny fails\n")})
				}
				commits = append(commits, topologyEdit{l + "-product-makes-" + lj + "-panic", was + "it panics", setAt(at+"panic", lj+" panics\n")})
				if tp.kinds[j-1] == 'B' {
					break
				}
			}
		}
		fails = append(fails, topologyEdit{l + "-panics", l + " panics", setAt(tree+"panic", l+" panics\n")})
		commits = append(commits, topologyEdit{l + "-outcome-rejected", "the archive commits " + l + "'s outcome as rejected", setAt("out/"+l+"/outcome", "rejected\n")})
	}
	commits = append(commits,
		topologyEdit{"list-files-stale", "the archive commits list's files stale", setAt("out/list/files.txt", "0 files\n")},
		topologyEdit{"list-stray", "the archive commits a file of list's that it doesn't make", setAt("out/list/stray.txt", "a stray\n")})
	var b strings.Builder
	at := topologyTree(tp.kinds, 0, n+1)
	fmt.Fprintf(&b, "A chain of lifts, each following its tree.\n-- in/%sA1 --\n1\n-- in/%sB1 --\n2\n", at, at)
	for _, f := range fails {
		for _, c := range commits {
			name := "f-" + f.name + "-c-" + c.name
			prefix := packedPrefix(name) + "/"
			// A variant changes a file of the base: each adds its name, in a
			// tree no lift reads, which no bytes lift's product holds.
			fmt.Fprintf(&b, "-- %sabout --\n%s, and %s.\n-- %sin/scenario/name --\n%s\n", prefix, strings.ToUpper(f.does[:1])+f.does[1:], c.does, prefix, name)
			g.scenarios = append(g.scenarios, gridScenario{name: name, edits: []gridEdit{{f.name, f.fn(prefix)}, {c.name, c.fn(prefix)}}})
		}
	}
	g.prep = func(s *sandbox) {
		s.write(g.archive, []byte(b.String()), "the topology's base, and a variant for each scenario")
		if two {
			s.write("b/corpus_test.go", []byte(packageTestFile), "./b is a package with tests")
		}
	}
	return g
}
