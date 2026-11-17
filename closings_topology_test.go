package hypercase_test

import (
	"fmt"
	"io"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/registry"
	"github.com/sdboyer/jenny"
)

// The topology grids are TestClosings' grids made from every shape a chain
// of lifts takes, where each hand-picked grid takes one. A topology is a
// chain of one to maxChain lifts from the corpus type Top0, of
// testdata/top0/, each a bytes lift (B) or a stage (N), then the suite list
// over the type the chain ends in, and the package that owns each of them:
// the module's root package, a, or ./b, with a owning one at least, and
// every stage. A package uses the lifts on its columns' paths that it
// doesn't own.
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
// transformGrid gives one chain the transform t, whose variant is one to
// an archive, so its scenarios each run on an archive of its own.

// maxChain is the longest chain a topology has. On a cell's path, a
// closing tells apart the lift that fails, F, the last bytes lift at or
// before it, R, which the update reruns, the bytes lift whose committed
// product the scenario edits, E, the last bytes lift, L, and the last one
// the cell's package doesn't own, U. Chains of up to four lifts stand
// them, and the cell, in every order but the eight that ordersPastMaxChain
// lists, which take five lifts, and TestTopologyOrders checks that the
// grids give each of the others. Five lifts would give every order, in
// nearly five times the scenarios, more than CI's time affords.
const maxChain = 4

// The types a topology's lifts take and make: Top0, the corpus type, and at
// position k, TopkB, the product of a bytes lift, a corpus type of
// testdata/topk/, which holds no archive, or *TopkN, a stage's.
type (
	Top0  struct{ fs.FS }
	Top1B struct{ fs.FS }
	Top2B struct{ fs.FS }
	Top3B struct{ fs.FS }
	Top4B struct{ fs.FS }
	Top1N struct{ view fs.FS }
	Top2N struct{ view fs.FS }
	Top3N struct{ view fs.FS }
	Top4N struct{ view fs.FS }
)

// A topologyValue is a value a topology's lift or list takes, which gives
// the input view it comes from.
type topologyValue interface{ topologyView() fs.FS }

func (v Top0) topologyView() fs.FS   { return v.FS }
func (v Top1B) topologyView() fs.FS  { return v.FS }
func (v Top2B) topologyView() fs.FS  { return v.FS }
func (v Top3B) topologyView() fs.FS  { return v.FS }
func (v Top4B) topologyView() fs.FS  { return v.FS }
func (v *Top1N) topologyView() fs.FS { return v.view }
func (v *Top2N) topologyView() fs.FS { return v.view }
func (v *Top3N) topologyView() fs.FS { return v.view }
func (v *Top4N) topologyView() fs.FS { return v.view }

// A topology is a chain's kinds, a letter for each lift, B or N, and its
// owners, a letter for each lift and then list's, a or b. Where transform
// is set, Top0 has the transform t, which a owns.
type topology struct {
	kinds, owners string
	transform     bool
}

// topologies are every chain of one to maxChain lifts, crossed with every
// assignment of its bytes lifts and list to a and b in which a owns one
// at least, with a owning every stage. A stage's owner moves only the
// stage's own cell: a package runs the stages on its columns' paths in
// both runs, owned or used, where it reads a bytes lift's committed
// product in a normal run unless it owns the lift. And where b owns a
// stage, swapping a and b gives a topology here in which a does, whose
// cells stand to the bytes lifts' owners as the stage's did.
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
			o := owners.String()
			if strings.Contains(o, "a") && !stagesOwnedByB(kinds, o) {
				out = append(out, topology{kinds: kinds, owners: o})
			}
		}
	}
	return out
}

// stagesOwnedByB reports whether owners gives b a stage of kinds.
func stagesOwnedByB(kinds, owners string) bool {
	for i := range len(kinds) {
		if kinds[i] == 'N' && owners[i] == 'b' {
			return true
		}
	}
	return false
}

func (tp topology) name() string {
	name := "topology-" + tp.kinds + "-" + tp.owners
	if tp.transform {
		name += "-t"
	}
	return name
}

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
	tp := topology{kinds: parts[0]}
	switch {
	case len(parts) == 4 && parts[2] == "t":
		tp.transform = true
		parts = slices.Delete(parts, 2, 3)
	case len(parts) != 3:
		panic("selftest: no topology registration named " + name)
	}
	if len(parts[2]) != 1 {
		panic("selftest: no topology registration named " + name)
	}
	tp.owners = parts[1]
	return func() { tp.register(parts[2][0]) }, true
}

// register registers the package pkg, a or b, of the topology: the corpus
// types and properties of its chain, the columns it owns, and the lifts it
// uses, those on its columns' paths that it doesn't own.
func (tp topology) register(pkg byte) {
	top0 := decl.Type[Top0]{Dir: "top0", Description: "Trees of instructions, under in/<lift>/, that a chain of lifts follows, and the data, under in/data/ or below a bytes lift's tree."}
	if tp.transform {
		top0.Transforms = []decl.Transform[Top0]{topologyTransform}
	}
	items := []any{project, top0}
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
	if tp.transform && pkg == 'a' {
		owned = append(owned, topologyTransform)
	}
	items = append(items, own(owned...))
	if len(used) > 0 {
		items = append(items, hypercase.Use(used...))
	}
	hypercase.Register(items...)
}

// topologyTransform is the transform t of Top0: it makes the base's input
// view with t/made added, and the base's t/add/ tree laid over it, so that
// a scenario that adds to that tree makes a lift fail on what t makes, and
// not on the inputs the archive holds for its variant.
var topologyTransform = decl.Transform[Top0]{
	Name:        "t",
	Description: "Adds t/made to the base, and lays the base's t/add/ tree over it.",
	Jenny: jenny.For[Top0]().New("t", func(base Top0) (jenny.Files, error) {
		paths, err := archive.Paths(base)
		if err != nil {
			return nil, err
		}
		files := map[string][]byte{"t/made": []byte("made by t\n")}
		for _, p := range paths {
			data, err := fs.ReadFile(base, p)
			if err != nil {
				return nil, err
			}
			files[p] = data
			if rel, ok := strings.CutPrefix(p, "t/add/"); ok {
				files[rel] = data
			}
		}
		var out jenny.Files
		for _, p := range slices.Sorted(maps.Keys(files)) {
			out = append(out, jenny.NewFile(p, files[p]))
		}
		return out, nil
	}),
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
	case 3:
		return decl.Type[Top3B]{Dir: "top3", Description: desc}
	}
	return decl.Type[Top4B]{Dir: "top4", Description: desc}
}

// column is the topology's column at index i: the lift at position i+1, or
// list, past the chain.
func (tp topology) column(i int) any {
	if i == len(tp.kinds) {
		return topologyList(tp.kinds)
	}
	k := i + 1
	if k == 1 {
		return topologyLift[Top0](tp.kinds, k)
	}
	switch b := tp.kinds[k-2] == 'B'; {
	case k == 2 && b:
		return topologyLift[Top1B](tp.kinds, k)
	case k == 2:
		return topologyLift[*Top1N](tp.kinds, k)
	case k == 3 && b:
		return topologyLift[Top2B](tp.kinds, k)
	case k == 3:
		return topologyLift[*Top2N](tp.kinds, k)
	case b:
		return topologyLift[Top3B](tp.kinds, k)
	}
	return topologyLift[*Top3N](tp.kinds, k)
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
	case k == 3 && b:
		return topologyBytes[In, Top3B](kinds, k)
	case k == 3:
		return topologyStage[In](k, func(v fs.FS) *Top3N { return &Top3N{v} })
	case b:
		return topologyBytes[In, Top4B](kinds, k)
	}
	return topologyStage[In](k, func(v fs.FS) *Top4N { return &Top4N{v} })
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
	case len(kinds) == 3 && b:
		return topologySuite[Top3B]()
	case len(kinds) == 3:
		return topologySuite[*Top3N]()
	case b:
		return topologySuite[Top4B]()
	}
	return topologySuite[*Top4N]()
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

// topologyGrids are the topology grids, one for each topology, and
// transformGrid.
func topologyGrids() []closingGrid {
	var gs []closingGrid
	for _, tp := range topologies() {
		gs = append(gs, tp.grid())
	}
	return append(gs, transformGrid())
}

// transformGrid is the grid of the topology NB, a stage, l1, then a bytes
// lift, l2, then list, which a owns, with the transform t. Its scenarios
// run on archives of their own, each of which the first update writes,
// with t's variant. One that adds l1's error to the base's t/add/ tree
// makes l1 reject what t makes, which the update stages the variant on,
// and not the inputs the archive holds for it: the update blocks the
// variant's cells past l1 before every bytes lift on their paths. Each is
// crossed with an edit of what the archive commits for the variant.
func transformGrid() closingGrid {
	tp := topology{kinds: "NB", owners: "aaa", transform: true}
	g := closingGrid{name: tp.name(), archive: "testdata/top0/base.txtar", pkgs: []childRun{{pkg: "", registration: tp.registration('a')}}}
	at := topologyTree(tp.kinds, 0, len(tp.kinds)+1)
	base := fmt.Sprintf("A chain of lifts, each following its tree.\n-- in/%sA1 --\n1\n-- in/%sB1 --\n2\n", at, at)
	g.prep = func(s *sandbox) { s.write(g.archive, []byte(base), "the topology's base") }
	const v = "transform/t/"
	fails := []gridEdit{
		{"no lift fails", func(*archive.Archive) {}},
		{"t makes l1 reject", insert("in/t/add/l1/error", "l1 rejects the case\n", "out/")},
	}
	commits := []gridEdit{
		{"the archive commits what the first update wrote", func(*archive.Archive) {}},
		{"l1's outcome rejected", set(v+"out/l1/outcome", "rejected\n")},
		{"l2's product with a file changed", set(v+"via/l2/"+topologyTree(tp.kinds, 2, len(tp.kinds)+1)+"A1", "9\n")},
		{"list's files stale", set(v+"out/list/files.txt", "0 files\n")},
	}
	g.scenarios = cross(fails, commits)
	return g
}

// A topologyEdit is one side of a topology's scenario: an edit of the
// variant whose sections start with a prefix, which says what it does.
// at is the position of the lift it makes fail, for a way a lift fails,
// and of the bytes lift whose committed product it edits, for an edit of
// what the archive commits, and 0 for an edit that does neither.
type topologyEdit struct {
	name, does string
	at         int
	fn         func(prefix string) func(a *archive.Archive)
}

// A topologyScenario is one of a topology's scenarios: a way one lift
// fails, under the variant's in/, crossed with an edit of what the archive
// commits for the variant.
type topologyScenario struct {
	name         string
	fail, commit topologyEdit
}

// scenarios are the topology's scenarios: each way the hand-picked grids
// make a lift on the chain fail, and none, crossed with each edit of what
// the archive commits that they make, and none.
func (tp topology) scenarios() []topologyScenario {
	n := len(tp.kinds)
	none := func(string) func(*archive.Archive) { return func(*archive.Archive) {} }
	setAt := func(name, data string) func(string) func(*archive.Archive) {
		return func(prefix string) func(*archive.Archive) { return set(prefix+name, data) }
	}
	fails := []topologyEdit{{"none", "no lift fails", 0, none}}
	commits := []topologyEdit{{"none", "the archive commits what the first update wrote", 0, none}}
	data := func(k int) string { return fmt.Sprintf("via/l%d/%s", k, topologyTree(tp.kinds, k, n+1)) }
	for k := 1; k <= n; k++ {
		l := fmt.Sprintf("l%d", k)
		tree := "in/" + topologyTree(tp.kinds, 0, k)
		stated := func(prefix string) func(*archive.Archive) {
			return seq(set(prefix+"properties", fmt.Sprintf("p%d\n", k)), set(prefix+tree+"error", l+" rejects the case\n"))
		}
		fails = append(fails,
			topologyEdit{l + "-rejects-stated", fmt.Sprintf("%s rejects the case, which states p%d", l, k), k, stated},
			topologyEdit{l + "-rejects", l + " rejects the case, which states no property", k, setAt(tree+"error", l+" rejects the case\n")})
		if tp.kinds[k-1] == 'B' {
			short := func(prefix string) func(*archive.Archive) { return drop(prefix + data(k) + "A1") }
			fails = append(fails, topologyEdit{l + "-faults", l + "'s jenny fails", k, setAt(tree+"fail", l+"'s jenny fails\n")})
			commits = append(commits,
				topologyEdit{l + "-product-changed", "the archive commits " + l + "'s product with a file changed", k, setAt(data(k)+"A1", "9\n")},
				topologyEdit{l + "-product-short", "the archive commits " + l + "'s product short of a file", k, short},
				topologyEdit{l + "-product-extra", "the archive commits " + l + "'s product with a file it doesn't make", k, setAt(data(k)+"Z9", "9\n")})
			// A lift that reads the product, up to the next bytes lift, fails on
			// what it commits, in each way, where it doesn't on what lk makes.
			for j := k + 1; j <= n; j++ {
				lj, at := fmt.Sprintf("l%d", j), fmt.Sprintf("via/l%d/%s", k, topologyTree(tp.kinds, k, j))
				was := "the archive commits " + l + "'s product with " + lj + "'s tree saying "
				commits = append(commits, topologyEdit{l + "-product-makes-" + lj + "-reject", was + "it rejects the case", k, setAt(at+"error", lj+" rejects the case\n")})
				if tp.kinds[j-1] == 'B' {
					commits = append(commits, topologyEdit{l + "-product-makes-" + lj + "-fault", was + "its jenny fails", k, setAt(at+"fail", lj+"'s jenny fails\n")})
				}
				commits = append(commits, topologyEdit{l + "-product-makes-" + lj + "-panic", was + "it panics", k, setAt(at+"panic", lj+" panics\n")})
				if tp.kinds[j-1] == 'B' {
					break
				}
			}
		}
		fails = append(fails, topologyEdit{l + "-panics", l + " panics", k, setAt(tree+"panic", l+" panics\n")})
		commits = append(commits, topologyEdit{l + "-outcome-rejected", "the archive commits " + l + "'s outcome as rejected", 0, setAt("out/"+l+"/outcome", "rejected\n")})
	}
	commits = append(commits,
		topologyEdit{"list-files-stale", "the archive commits list's files stale", 0, setAt("out/list/files.txt", "0 files\n")},
		topologyEdit{"list-stray", "the archive commits a file of list's that it doesn't make", 0, setAt("out/list/stray.txt", "a stray\n")})
	var out []topologyScenario
	for _, f := range fails {
		for _, c := range commits {
			out = append(out, topologyScenario{"f-" + f.name + "-c-" + c.name, f, c})
		}
	}
	return out
}

// grid is the topology's grid. Each scenario is a hand-written variant of
// the base, which the first update writes as it writes the base, and which
// the scenario then edits.
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
	var b strings.Builder
	at := topologyTree(tp.kinds, 0, n+1)
	fmt.Fprintf(&b, "A chain of lifts, each following its tree.\n-- in/%sA1 --\n1\n-- in/%sB1 --\n2\n", at, at)
	for _, sc := range tp.scenarios() {
		f, c := sc.fail, sc.commit
		prefix := packedPrefix(sc.name) + "/"
		// A variant changes a file of the base: each adds its name, in a
		// tree no lift reads, which no bytes lift's product holds.
		fmt.Fprintf(&b, "-- %sabout --\n%s, and %s.\n-- %sin/scenario/name --\n%s\n", prefix, strings.ToUpper(f.does[:1])+f.does[1:], c.does, prefix, sc.name)
		g.scenarios = append(g.scenarios, gridScenario{name: sc.name, edits: []gridEdit{{f.name, f.fn(prefix)}, {c.name, c.fn(prefix)}}})
	}
	g.prep = func(s *sandbox) {
		s.write(g.archive, []byte(b.String()), "the topology's base, and a variant for each scenario")
		if two {
			s.write("b/corpus_test.go", []byte(packageTestFile), "./b is a package with tests")
		}
	}
	return g
}

// pathOrder words the order in which the lifts a closing tells apart
// stand on the path of a cell, and the cell, c, on a chain of kinds: the
// cell is the column at position c, list's past the chain. The lift at
// position f fails, F, for f other than 0, and R is the last bytes lift at
// or before it, where it is on the cell's path; E is the bytes lift at
// position e, whose committed product the scenario edits, for e other than
// 0; L is the last bytes lift on the cell's path, and U the last there
// that unowned says the cell's package doesn't own. A lift past the cell
// is none of its: neither the cell's run nor what the archive commits for
// it reads what that lift makes. Labels at one position are joined by =,
// in their order, and positions by <, along the chain.
func pathOrder(kinds string, c, f, e int, unowned func(j int) bool) string {
	at := map[int][]string{c: {"c"}}
	lastBytes := func(j int) int {
		for ; j >= 1 && kinds[j-1] != 'B'; j-- {
		}
		return j
	}
	if f > 0 && f <= c {
		at[f] = append(at[f], "F")
		if r := lastBytes(f); f < c && r > 0 {
			at[r] = append(at[r], "R")
		}
	}
	if e > 0 && e <= c {
		at[e] = append(at[e], "E")
	}
	if l := lastBytes(c - 1); l > 0 {
		at[l] = append(at[l], "L")
	}
	for j := lastBytes(c - 1); j > 0; j = lastBytes(j - 1) {
		if unowned(j) {
			at[j] = append(at[j], "U")
			break
		}
	}
	var groups []string
	for _, p := range slices.Sorted(maps.Keys(at)) {
		slices.Sort(at[p])
		groups = append(groups, strings.Join(at[p], "="))
	}
	return strings.Join(groups, "<")
}

// An orderChain is the shortest chain that gives an order, as
// chainOrders finds it: its kinds, the cell's position, the failing lift's,
// the edited product's, and the last unowned bytes lift's, 0 for none.
type orderChain struct {
	kinds         string
	c, f, e, last int
}

// chainOrders gives each order pathOrder words on some chain of up to
// longest lifts, of each kind, for each cell, failing lift or none, edited
// product or none, and last bytes lift on the cell's path that its package
// doesn't own or none, with the shortest such chain.
func chainOrders(longest int) map[string]orderChain {
	out := map[string]orderChain{}
	for n := 1; n <= longest; n++ {
		for mask := range 1 << n {
			var b strings.Builder
			var bytes []int
			for j := range n {
				b.WriteByte("NB"[mask>>j&1])
				if mask>>j&1 == 1 {
					bytes = append(bytes, j+1)
				}
			}
			kinds := b.String()
			for c := 1; c <= n+1; c++ {
				for f := range n + 1 {
					for _, e := range append([]int{0}, bytes...) {
						for _, u := range append([]int{0}, bytes...) {
							if u >= c {
								continue
							}
							o := pathOrder(kinds, c, f, e, func(j int) bool { return j == u })
							if _, ok := out[o]; !ok {
								out[o] = orderChain{kinds, c, f, e, u}
							}
						}
					}
				}
			}
		}
	}
	return out
}

// ordersPastMaxChain are the orders, as pathOrder words them, that only a
// chain of more than maxChain lifts gives.
var ordersPastMaxChain = []string{
	"E<R<F<U<L<c",
	"E<U<R<F<L<c",
	"R<F<E<U<L<c",
	"R<F<U<E<L<c",
	"R<F<U<L<E=c",
	"U<E<R<F<L<c",
	"U<R<F<E<L<c",
	"U<R<F<L<E=c",
}

// TestTopologyOrders: the topology grids give each order, as pathOrder
// words it, in which a chain of up to maxChain lifts stands the lifts a
// closing tells apart on a cell's path, and the cell: some scenario of
// some topology gives a cell whose path stands them so. ordersPastMaxChain
// lists the orders only a longer chain gives. An order labels five lifts,
// and a lift no label names can be dropped from a chain with the order
// kept, so chains of six lifts give every order any chain gives: the sixth
// lift adds none, as the test checks.
func TestTopologyOrders(t *testing.T) {
	t.Parallel()
	const longest = 6
	possible := chainOrders(longest)
	realized := map[string]string{} // each order, and the first scenario and cell that gives it
	for _, tp := range topologies() {
		for _, sc := range tp.scenarios() {
			for i := range tp.owners {
				o := pathOrder(tp.kinds, i+1, sc.fail.at, sc.commit.at, func(j int) bool { return tp.owners[j-1] != tp.owners[i] })
				if _, ok := realized[o]; !ok {
					realized[o] = fmt.Sprintf("%s, %s, the cell of the column at %d", tp.name(), sc.name, i+1)
				}
			}
		}
	}
	var past []string
	for _, o := range slices.Sorted(maps.Keys(possible)) {
		ch := possible[o]
		_, ok := realized[o]
		switch n := len(ch.kinds); {
		case n == longest:
			t.Errorf("only a chain of %d lifts gives the order %s, %+v, so a longer one may give others: an order labels five lifts", longest, o, ch)
		case n > maxChain:
			past = append(past, o)
			if ok {
				t.Errorf("%s gives the order %s, which only a chain of %d lifts gives", realized[o], o, n)
			}
		case !ok:
			t.Errorf("no scenario of the topology grids gives a cell whose path stands the lifts in the order %s, which a chain of %d lifts gives: %+v", o, n, ch)
		}
	}
	for _, o := range slices.Sorted(maps.Keys(realized)) {
		if _, ok := possible[o]; !ok {
			t.Errorf("%s gives the order %s, which no chain gives", realized[o], o)
		}
	}
	if !slices.Equal(past, ordersPastMaxChain) {
		t.Errorf("the orders only a chain of more than %d lifts gives are\n%q\nand ordersPastMaxChain lists\n%q", maxChain, past, ordersPastMaxChain)
	}
	t.Logf("the topology grids give %d of the %d orders any chain gives", len(realized), len(possible))
}

// TestTopologyPaths: over every topology, and transformGrid's, the
// registry of each package records each column's paths as the chain gives
// them: one from testdata/top0/, and one from the corpus type of each
// bytes lift before the column, each naming the lifts between, nearest the
// column first. And a normal run of each package passes after the first
// update, so the root-package check finds no package that reaches a lift
// it uses otherwise than the lift's owner: in a topology, a package
// registers every corpus type of the chain, and uses each lift on its
// columns' paths that it doesn't own.
func TestTopologyPaths(t *testing.T) {
	t.Parallel()
	tps := append(topologies(), topology{kinds: "NB", owners: "aaa", transform: true})
	for _, tp := range tps {
		t.Run(tp.name(), func(t *testing.T) {
			t.Parallel()
			n := len(tp.kinds)
			name := func(j int) string {
				if j == n+1 {
					return "list"
				}
				return fmt.Sprintf("l%d", j)
			}
			// want is the paths of the column at position j, list's past
			// the chain.
			want := func(j int) []registry.Path {
				var ps []registry.Path
				for k := range j {
					if k > 0 && tp.kinds[k-1] != 'B' {
						continue
					}
					p := registry.Path{Corpus: fmt.Sprintf("top%d", k), Lifts: []string{}}
					for m := j - 1; m > k; m-- {
						p.Lifts = append(p.Lifts, name(m))
					}
					ps = append(ps, p)
				}
				return ps
			}
			s := newSandbox(t, "")
			at := topologyTree(tp.kinds, 0, n+1)
			s.write("testdata/top0/base.txtar", fmt.Appendf(nil, "A chain of lifts, each following its tree.\n-- in/%sA1 --\n1\n-- in/%sB1 --\n2\n", at, at), "the topology's base")
			g := closingGrid{pkgs: []childRun{{pkg: "", registration: tp.registration('a')}}}
			regs := []string{"testdata/hypercase.json"}
			if strings.Contains(tp.owners, "b") {
				s.write("b/corpus_test.go", []byte(packageTestFile), "./b is a package with tests")
				g.pkgs = append(g.pkgs, childRun{pkg: "b", registration: tp.registration('b')})
				regs = append(regs, "b/testdata/hypercase.json")
			}
			for _, update := range []bool{true, false} {
				for i, r := range g.runAll(s, update, "") {
					if !r.ok() {
						t.Fatalf("the run of %q, with update %v, failed:\n%s", g.pkgs[i].pkg, update, s.shown(r))
					}
				}
			}
			for _, reg := range regs {
				g, err := registry.Parse(s.read(reg))
				if err != nil {
					t.Fatal(err)
				}
				got := map[string][]registry.Path{}
				for _, l := range g.Lifts {
					got[l.Name] = l.Paths
				}
				for _, c := range g.Suites {
					got[c.Name] = c.Paths
				}
				if len(got) == 0 {
					t.Errorf("%s records no column", reg)
				}
				for j := 1; j <= n+1; j++ {
					if ps, ok := got[name(j)]; ok && !reflect.DeepEqual(ps, want(j)) {
						t.Errorf("%s records %s's paths as %+v, and the chain gives %+v", reg, name(j), ps, want(j))
					}
				}
			}
		})
	}
}
