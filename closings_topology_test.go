package hypercase_test

import (
	"fmt"
	"io"
	"io/fs"
	"maps"
	"path"
	"reflect"
	"regexp"
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
// is set, Top0 has the transform t, which a owns. starts gives, for a and
// then b, the position of the first lift the package registers, after the
// corpus type it starts from: 1, after Top0, or k+1, after the product of
// the bytes lift at k, and 0 for 1. Where shadow is set, b registers, in
// place of the stage at that position, which a owns, a copy of it it
// owns, named for it with an x: l2x for l2. Where third is set, a third
// package, ./c, starts at that position, as starts gives it, and owns
// listc, a copy of list, using every lift from its start, a's stage at
// the shadow's position among them.
type topology struct {
	kinds, owners string
	transform     bool
	starts        [2]int
	shadow        int
	third         int
}

// start is the position of the first lift the package pkg, a, b or c,
// registers.
func (tp topology) start(pkg byte) int {
	if pkg == 'c' {
		return tp.third
	}
	return max(tp.starts[pkg-'a'], 1)
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
	if tp.starts != [2]int{} {
		name += fmt.Sprintf("-s%d%d", tp.start('a'), tp.start('b'))
	}
	if tp.shadow > 0 {
		name += fmt.Sprintf("-x%d", tp.shadow)
	}
	if tp.third > 0 {
		name += fmt.Sprintf("-c%d", tp.third)
	}
	return name
}

// registration names the registration of the package pkg, a, b or c,
// which topologyRegistration reads.
func (tp topology) registration(pkg byte) string { return tp.name() + "-" + string(pkg) }

// topologyRegistration is the registration a name gives, as a topology's
// or a branching's registration names it.
func topologyRegistration(name string) (func(), bool) {
	if rest, ok := strings.CutPrefix(name, "branching-"); ok {
		parts := strings.Split(rest, "-")
		if len(parts) != 3 || len(parts[0]) != 6 || len(parts[1]) != 2 || len(parts[2]) != 1 {
			panic("selftest: no branching registration named " + name)
		}
		br := branching{owners: parts[0], late: [2]bool{parts[1][0] == 'l', parts[1][1] == 'l'}}
		return func() { br.register(parts[2][0]) }, true
	}
	rest, ok := strings.CutPrefix(name, "topology-")
	if !ok {
		return nil, false
	}
	parts := strings.Split(rest, "-")
	if len(parts) < 3 || len(parts[len(parts)-1]) != 1 {
		panic("selftest: no topology registration named " + name)
	}
	tp := topology{kinds: parts[0], owners: parts[1]}
	for _, p := range parts[2 : len(parts)-1] {
		switch {
		case p == "t":
			tp.transform = true
		case len(p) == 3 && p[0] == 's':
			tp.starts = [2]int{int(p[1] - '0'), int(p[2] - '0')}
		case len(p) == 2 && p[0] == 'x':
			tp.shadow = int(p[1] - '0')
		case len(p) == 2 && p[0] == 'c':
			tp.third = int(p[1] - '0')
		default:
			panic("selftest: no topology registration named " + name)
		}
	}
	pkg := parts[len(parts)-1][0]
	return func() { tp.register(pkg) }, true
}

// register registers the package pkg, a, b or c, of the topology: the
// properties of its chain, the corpus type it starts from and each bytes
// lift's product past it, the columns it owns, and the lifts it uses,
// those on its columns' paths from its start that it doesn't own, but
// for b the stage its shadow replaces, whose copy it owns. c owns listc.
func (tp topology) register(pkg byte) {
	start := tp.start(pkg)
	items := []any{project}
	if start == 1 {
		top0 := topologyTop0
		if tp.transform {
			top0.Transforms = []decl.Transform[Top0]{topologyTransform}
		}
		items = append(items, top0)
	} else {
		items = append(items, topologyType(start-1))
	}
	if pkg == 'a' {
		items = append(items, rootPkg)
	}
	for k := 1; k <= len(tp.kinds); k++ {
		items = append(items, topologyProperty(k))
		if k >= start && tp.kinds[k-1] == 'B' {
			items = append(items, topologyType(k))
		}
	}
	last := tp.last(pkg) - 1
	var owned, used []any
	for i := range tp.owners {
		switch col := tp.column(i); {
		case pkg == 'c' && i == len(tp.kinds):
			owned = append(owned, renamed(col, "listc"))
		case tp.owners[i] == pkg:
			owned = append(owned, col)
		case pkg == 'b' && i+1 == tp.shadow:
			owned = append(owned, renamed(col, fmt.Sprintf("l%dx", i+1)))
		case i < last && i+1 >= start:
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

// topologyTop0 is the corpus type Top0, of testdata/top0/.
var topologyTop0 = decl.Type[Top0]{Dir: "top0", Description: "Trees of instructions, under in/<lift>/, that a chain of lifts follows, and the data, under in/data/ or below a bytes lift's tree."}

// renamed is a copy of a lift declared as a decl.Lift, or of a suite, under
// another name.
func renamed(lift any, name string) any {
	v := reflect.New(reflect.TypeOf(lift)).Elem()
	v.Set(reflect.ValueOf(lift))
	v.FieldByName("Name").SetString(name)
	return v.Interface()
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
	var product []decl.Output
	for j := k + 1; j <= len(kinds); j++ {
		product = append(product, decl.Output{Pattern: topologyTree(kinds, k, j) + "*", Description: fmt.Sprintf("What l%d does, as its tree says.", j)})
	}
	product = append(product, decl.Output{Pattern: topologyTree(kinds, k, len(kinds)+1) + "*", Description: "The data list lists."})
	return topologyBytesOf[In, Out](k, product)
}

// topologyBytesOf is the bytes lift at position k, whose product holds
// the files product declares.
func topologyBytesOf[In topologyValue, Out ~struct{ fs.FS }](k int, product []decl.Output) decl.Lift[In, Out] {
	name := fmt.Sprintf("l%d", k)
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

// maxUsedChain is the longest chain usedPathTopologies gives.
const maxUsedChain = 3

// usedPathTopologies are the topologies, of chains of one to maxUsedChain
// lifts, in which packages reach the lifts they use in every way the
// root-package check tells apart, and topologies doesn't give: a and b
// each own a column, a package starts from Top0 or from the product of
// any bytes lift before the first column it owns, owns or uses each lift
// past its start up to its last column, and finds the owner of each lift
// it uses starting at or before it; and b may register, in place of a
// stage a owns that b would use, a copy of it it owns, its shadow. So a
// package uses a lift from a corpus its owner doesn't run it over where
// the user starts earlier, and by another path where one of them reaches
// it through the shadow, and each beside topologies that differ in a start
// or a shadow, which the check accepts.
func usedPathTopologies() []topology {
	in := map[topology]bool{}
	for _, tp := range topologies() {
		in[tp] = true
	}
	var out []topology
	for n := 1; n <= maxUsedChain; n++ {
		for kmask := range 1 << n {
			var kb strings.Builder
			for i := range n {
				kb.WriteByte("BN"[kmask>>i&1])
			}
			kinds := kb.String()
			starts := []int{1}
			for k := 1; k <= n; k++ {
				if kinds[k-1] == 'B' {
					starts = append(starts, k+1)
				}
			}
			for omask := range 1 << (n + 1) {
				var ob strings.Builder
				for i := range n + 1 {
					ob.WriteByte("ab"[omask>>i&1])
				}
				owners := ob.String()
				if !strings.Contains(owners, "a") || !strings.Contains(owners, "b") {
					continue
				}
				for _, sa := range starts {
					for _, sb := range starts {
						tp := topology{kinds: kinds, owners: owners, starts: [2]int{sa, sb}}
						if !tp.usesOwned() {
							continue
						}
						shadows := []int{0}
						last := strings.LastIndexByte(owners, 'b') + 1
						for j := sb; j < last && j <= n; j++ {
							if kinds[j-1] == 'N' && owners[j-1] == 'a' {
								shadows = append(shadows, j)
							}
						}
						for _, x := range shadows {
							tp.shadow = x
							if sa == 1 && sb == 1 && x == 0 {
								tp.starts = [2]int{}
								if in[tp] {
									continue
								}
							}
							out = append(out, tp)
						}
					}
				}
			}
		}
	}
	return out
}

// usesOwned reports whether a and b each start at or before the first
// column they own, and each lift a package uses has an owner that starts
// at or before it.
func (tp topology) usesOwned() bool {
	for _, p := range []byte("ab") {
		if tp.start(p) > strings.IndexByte(tp.owners, p)+1 {
			return false
		}
	}
	for _, p := range tp.pkgs() {
		for k := tp.start(p); k < tp.last(p); k++ {
			if o := tp.owners[k-1]; o != p && tp.start(o) > k {
				return false
			}
		}
	}
	return true
}

// last is the position of the last column the package pkg owns: for c,
// listc's, past the chain.
func (tp topology) last(pkg byte) int {
	if pkg == 'c' {
		return len(tp.kinds) + 1
	}
	return strings.LastIndexByte(tp.owners, pkg) + 1
}

// thirdPackageCandidates are the topologies of topologies, of chains of
// up to maxUsedChain lifts, and of usedPathTopologies, with ./c added,
// from each start from which the lifts it uses have owners that start at
// or before them.
func thirdPackageCandidates() []topology {
	var bases []topology
	for _, tp := range topologies() {
		if len(tp.kinds) <= maxUsedChain {
			bases = append(bases, tp)
		}
	}
	var out []topology
	for _, tp := range append(bases, usedPathTopologies()...) {
		starts := []int{1}
		for k := 1; k <= len(tp.kinds); k++ {
			if tp.kinds[k-1] == 'B' {
				starts = append(starts, k+1)
			}
		}
		for _, c := range starts {
			tp.third = c
			if tp.usesOwned() {
				out = append(out, tp)
			}
		}
	}
	return out
}

// thirdPackageTopologies are thirdPackageCandidates, one for each
// signature c's lifts give, as usedSignature words it, which is all the
// root-package check reads to judge c's uses: it compares each use with
// the lift's owner's entry alone. So two topologies that give c one
// signature give c the same failures, as TestThirdPackageSignatures
// checks of refusals, and what a and b do, the topologies without c vary.
func thirdPackageTopologies() []topology {
	var out []topology
	seen := map[string]bool{}
	for _, tp := range thirdPackageCandidates() {
		if sig := tp.usedSignature('c'); !seen[sig] {
			seen[sig] = true
			out = append(out, tp)
		}
	}
	return out
}

// TestThirdPackageSignatures: of thirdPackageCandidates, those that give
// c one signature, as usedSignature words it, give c the same refusals,
// so thirdPackageTopologies, which keeps one of them, drops none of c's
// verdicts.
func TestThirdPackageSignatures(t *testing.T) {
	t.Parallel()
	byUser := func(tp topology, user string) []usedRefusal {
		return slices.DeleteFunc(tp.refusals(), func(r usedRefusal) bool { return r.user != user })
	}
	first := map[string]topology{}
	for _, tp := range thirdPackageCandidates() {
		sig := tp.usedSignature('c')
		kept, ok := first[sig]
		if !ok {
			first[sig] = tp
			continue
		}
		if got, want := byUser(tp, "./c"), byUser(kept, "./c"); !slices.Equal(got, want) {
			t.Errorf("%s and %s give c the signature\n%s\nand refusals gives c\n%s\nand\n%s", tp.name(), kept.name(), sig, refusalLines(got), refusalLines(want))
		}
	}
	t.Logf("%d candidates give %d signatures", len(thirdPackageCandidates()), len(first))
}

// usedSignature is what the root-package check reads of the lifts the
// package pkg uses: each one's name and owner, and its paths in pkg's
// registry and in its owner's, each as route shows it.
func (tp topology) usedSignature(pkg byte) string {
	var b strings.Builder
	for k := tp.start(pkg); k < tp.last(pkg); k++ {
		o := tp.owner(pkg, k)
		if o == pkg {
			continue
		}
		lift := fmt.Sprintf("l%d", k)
		fmt.Fprintf(&b, "%s, owned by %s:", lift, pkgDir(o))
		for _, c := range tp.corpora(pkg, k) {
			fmt.Fprintf(&b, " %s;", usedRoute(fmt.Sprintf("top%d", c), tp.lifts(pkg, k, c), lift))
		}
		b.WriteString(" owned:")
		for _, c := range tp.corpora(o, k) {
			fmt.Fprintf(&b, " %s;", usedRoute(fmt.Sprintf("top%d", c), tp.lifts(o, k, c), lift))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// corpora are the positions of the corpus types from which the package
// pkg reaches the column at position j: its start's, and each bytes
// lift's past it, before j.
func (tp topology) corpora(pkg byte, j int) []int {
	start := tp.start(pkg)
	out := []int{start - 1}
	for k := start; k < j; k++ {
		if tp.kinds[k-1] == 'B' {
			out = append(out, k)
		}
	}
	return out
}

// lifts names the lifts the package pkg reaches the column at position j
// through from the corpus type at position c, nearest the column first.
func (tp topology) lifts(pkg byte, j, c int) []string {
	out := []string{}
	for m := j - 1; m > c; m-- {
		name := fmt.Sprintf("l%d", m)
		if tp.owner(pkg, m) == 'b' && m == tp.shadow {
			name += "x"
		}
		out = append(out, name)
	}
	return out
}

// owner is the package that owns the lift at position k that the package
// pkg registers: b for its shadow.
func (tp topology) owner(pkg byte, k int) byte {
	if k == tp.shadow && pkg == 'b' {
		return 'b'
	}
	return tp.owners[k-1]
}

// A usedRefusal is one failure of the root-package check over a lift a
// package uses: the user's directory, the owner's, the corpus, the lift,
// the user's path from the corpus to the lift, and the owner's, or none
// where the owner doesn't run the lift over the corpus, each path as
// route shows it.
type usedRefusal struct{ user, owner, corpus, lift, mine, theirs string }

// usedRoute shows the path from the corpus to the lift through lifts,
// nearest the lift first, as route shows it.
func usedRoute(corpus string, lifts []string, lift string) string {
	s := "testdata/" + corpus + "/"
	for i := len(lifts) - 1; i >= 0; i-- {
		s += " → " + lifts[i]
	}
	return s + " → " + lift
}

// pkgDir is the directory of the package pkg, a, b or c, as the
// root-package check names it.
func pkgDir(pkg byte) string {
	if pkg == 'a' {
		return "."
	}
	return "./" + string(pkg)
}

// refusals are the root-package check's failures over the topology, from
// its description alone: one for each package that uses a lift, and each
// corpus it reaches the lift from that the lift's owner doesn't reach it
// from alike, as where the user starts earlier, or reaches it by another
// path, as through b's shadow.
func (tp topology) refusals() []usedRefusal {
	var out []usedRefusal
	for _, u := range tp.pkgs() {
		for k := tp.start(u); k < tp.last(u); k++ {
			o := tp.owner(u, k)
			if o == u {
				continue
			}
			lift := fmt.Sprintf("l%d", k)
			for _, c := range tp.corpora(u, k) {
				corpus := fmt.Sprintf("top%d", c)
				r := usedRefusal{pkgDir(u), pkgDir(o), corpus, lift, usedRoute(corpus, tp.lifts(u, k, c), lift), ""}
				switch {
				case !slices.Contains(tp.corpora(o, k), c):
				case !slices.Equal(tp.lifts(u, k, c), tp.lifts(o, k, c)):
					r.theirs = usedRoute(corpus, tp.lifts(o, k, c), lift)
				default:
					continue
				}
				out = append(out, r)
			}
		}
	}
	return out
}

// usedFailure is the first line of a failure of the root-package check
// over a lift a package uses: the lift, the user, the user's path and its
// corpus, the owner, and the corpus the owner doesn't run the lift over,
// or the owner's path.
var usedFailure = regexp.MustCompile(`the lift (\S+) is used by (\S+) on the path (testdata/(\w+)/(?: → \S+)*), and (\S+), which owns it, (?:doesn't run it over testdata/(\w+)/|runs it on the path (testdata/\w+/(?: → \S+)*)), so `)

// usedFailures reads the root-package check's failures over lifts
// packages use in out, each as a usedRefusal, sorted, and fails where one
// names its lift or its corpus twice otherwise, or where a failure it
// can't read speaks of a used lift.
func usedFailures(t *testing.T, out string) []usedRefusal {
	t.Helper()
	var got []usedRefusal
	ms := usedFailure.FindAllStringSubmatch(out, -1)
	for _, m := range ms {
		r := usedRefusal{m[2], m[5], m[4], m[1], m[3], m[7]}
		if m[6] != "" && m[6] != r.corpus {
			t.Errorf("a failure names the corpora testdata/%s/ and testdata/%s/: %q", r.corpus, m[6], m[0])
		}
		for _, p := range []string{r.mine, r.theirs} {
			if p != "" && (!strings.HasPrefix(p, "testdata/"+r.corpus+"/") || !strings.HasSuffix(p, " → "+r.lift)) {
				t.Errorf("a failure over %s from testdata/%s/ gives the path %s: %q", r.lift, r.corpus, p, m[0])
			}
		}
		got = append(got, r)
	}
	if n := strings.Count(out, " is used by "); n != len(ms) {
		t.Errorf("of the %d failures over used lifts, %d can be read:\n%s", n, len(ms), out)
	}
	sortRefusals(got)
	return got
}

func sortRefusals(rs []usedRefusal) {
	slices.SortFunc(rs, func(a, b usedRefusal) int { return strings.Compare(fmt.Sprint(a), fmt.Sprint(b)) })
}

// A pathTopology is what TestTopologyPaths runs: a topology, or a
// branching.
type pathTopology interface {
	name() string
	sandbox(t *testing.T) (*sandbox, closingGrid)
	refusals() []usedRefusal
	checkPaths(t *testing.T, s *sandbox)
}

// TestTopologyPaths: over every topology, transformGrid's,
// usedPathTopologies', thirdPackageTopologies' and the branchings', the
// registry of each package records each column's paths as the topology
// gives them: one from the corpus type the package starts from, and one
// from the product of each bytes lift past it before the column, each
// naming the lifts between, nearest the column first, with b's shadow for
// the stage it replaces. And after the first update, the root-package
// check fails exactly where refusals, worked out from the topology alone,
// says: the failures name the users, owners, corpora, lifts and paths it
// gives, each once. Every other run of a topology it accepts passes. In
// one it refuses, the user's run can fail too, where it reads the
// committed outcome of a bytes lift's cell that no package writes.
func TestTopologyPaths(t *testing.T) {
	t.Parallel()
	var tps []pathTopology
	family := map[string]string{} // each topology's family, by its name
	add := func(name string, tp pathTopology) {
		tps = append(tps, tp)
		family[tp.name()] = name
	}
	for _, tp := range append(topologies(), topology{kinds: "NB", owners: "aaa", transform: true}) {
		add("topologies and transformGrid", tp)
	}
	for _, tp := range usedPathTopologies() {
		add("usedPathTopologies", tp)
	}
	for _, tp := range thirdPackageTopologies() {
		add(fmt.Sprintf("thirdPackageTopologies, of %d candidates", len(thirdPackageCandidates())), tp)
	}
	for _, br := range branchings() {
		add("branchings", br)
	}
	counts := map[string][2]int{} // family → topologies, refused
	for _, tp := range tps {
		c := counts[family[tp.name()]]
		c[0]++
		if len(tp.refusals()) > 0 {
			c[1]++
		}
		counts[family[tp.name()]] = c
	}
	for _, f := range slices.Sorted(maps.Keys(counts)) {
		t.Logf("%s: %d topologies, of which the root-package check refuses %d", f, counts[f][0], counts[f][1])
	}
	for _, tp := range tps {
		t.Run(tp.name(), func(t *testing.T) {
			t.Parallel()
			s, g := tp.sandbox(t)
			want := tp.refusals()
			sortRefusals(want)
			for i, r := range g.runAll(s, false, "") {
				switch {
				case i == 0 && len(want) > 0:
					if r.ok() {
						t.Errorf("the root-package check accepts the topology, and should refuse\n%s", refusalLines(want))
					} else if got := usedFailures(t, string(r.out)); !slices.Equal(got, want) {
						t.Errorf("the root-package check refuses\n%s\nand should refuse\n%s\n%s", refusalLines(got), refusalLines(want), s.shown(r))
					}
				case !r.ok() && len(want) == 0:
					t.Errorf("the run of %q failed:\n%s", g.pkgs[i].pkg, s.shown(r))
				}
			}
			tp.checkPaths(t, s)
		})
	}
}

// TestTopologyUsedPathLeft: in the topology BN, ./b owns the bytes lift
// l1, and . the stage l2 and list, starting from Top1B, the product of
// l1; ./c, which owns listc, starts from Top0, and so reaches l2 from
// testdata/top0/ through l1, where . doesn't run l2. The root-package
// check fails once, and applying the second way the failure states ends
// it: ./c no longer reaches l2 from testdata/top0/, starting from Top1B,
// and its cells of top0's base, which no package writes then, are
// deleted. Every run passes then. TestChainRootCheckUsedPath applies the
// first way, and TestRootCheckUsedPath both ways a failure over a path
// other than the owner's states.
func TestTopologyUsedPathLeft(t *testing.T) {
	t.Parallel()
	tp := topology{kinds: "BN", owners: "baa", starts: [2]int{2, 1}, third: 1}
	want := []usedRefusal{{"./c", ".", "top0", "l2", "testdata/top0/ → l1 → l2", ""}}
	if got := tp.refusals(); !slices.Equal(got, want) {
		t.Fatalf("refusals gives\n%s\nwhere the test expects\n%s", refusalLines(got), refusalLines(want))
	}
	s, _ := tp.sandbox(t)
	r := s.normal(tp.registration('a'))
	s.fails(r, "the lift l2 is used by ./c on the path testdata/top0/ → l1 → l2, and ., which owns it, doesn't run it over testdata/top0/, so no package writes its cells of that corpus's cases, which ./c's cells past it are built through.\nA package that uses a lift reaches it from each corpus by the path the lift's owner does: either . runs l2 over testdata/top0/ by that path, or ./c doesn't reach l2 from testdata/top0/. If one holds already, a registry is stale: run FIXTURE_UPDATE_GOLDEN=1 go test ./..., which rewrites the registry of every package with tests.")
	if n := strings.Count(r.out, " is used by "); n != 1 {
		t.Errorf("the root-package check fails %d times on used lifts, and should fail once", n)
	}

	left := tp
	left.third = 2
	s.edit("testdata/top0/base.txtar", "delete ./c's cells of top0's base, which no package writes now", drop("out/listc/"))
	s.passes(s.run("c", left.registration('c'), true))
	s.passes(s.normal(left.registration('a')))
	s.passes(s.run("b", left.registration('b'), false))
	s.passes(s.run("c", left.registration('c'), false))
	if got := left.refusals(); len(got) > 0 {
		t.Errorf("refusals gives\n%s\nwhere ./c starts from Top1B", refusalLines(got))
	}
	left.checkPaths(t, s)
	s.done()
}

// refusalLines shows refusals, one to a line.
func refusalLines(rs []usedRefusal) string {
	var lines []string
	for _, r := range rs {
		lines = append(lines, fmt.Sprintf("  %+v", r))
	}
	return strings.Join(lines, "\n")
}

// sandbox is a sandbox holding the topology's packages and a base
// archive at each corpus type a package starts from, after the first
// update, and the grid of its packages.
func (tp topology) sandbox(t *testing.T) (*sandbox, closingGrid) {
	t.Helper()
	s := newSandbox(t, "")
	var g closingGrid
	for _, p := range tp.pkgs() {
		dir := ""
		if p != 'a' {
			dir = string(p)
			s.write(dir+"/corpus_test.go", []byte(packageTestFile), pkgDir(p)+" is a package with tests")
		}
		g.pkgs = append(g.pkgs, childRun{pkg: dir, registration: tp.registration(p)})
	}
	for _, c := range tp.startCorpora() {
		tree := topologyTree(tp.kinds, c, len(tp.kinds)+1)
		s.write(fmt.Sprintf("testdata/top%d/base.txtar", c), fmt.Appendf(nil, "A chain of lifts, each following its tree.\n-- in/%sA1 --\n1\n-- in/%sB1 --\n2\n", tree, tree), "the topology's base, where a package starts")
	}
	updateAll(t, s, g)
	return s, g
}

// updateAll runs the update of each package of g in s, and fails the test
// where one fails.
func updateAll(t *testing.T, s *sandbox, g closingGrid) {
	t.Helper()
	for i, r := range g.runAll(s, true, "") {
		if !r.ok() {
			t.Fatalf("the update of %q failed:\n%s", g.pkgs[i].pkg, s.shown(r))
		}
	}
}

// pkgs are the topology's packages: a, b where it owns a column, and c
// where third is set.
func (tp topology) pkgs() []byte {
	pkgs := []byte("a")
	if strings.Contains(tp.owners, "b") {
		pkgs = append(pkgs, 'b')
	}
	if tp.third > 0 {
		pkgs = append(pkgs, 'c')
	}
	return pkgs
}

// startCorpora are the positions of the corpus types the packages start
// from, each once.
func (tp topology) startCorpora() []int {
	var out []int
	for _, p := range tp.pkgs() {
		if c := tp.start(p) - 1; !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// registryColumns are the columns, suites and lifts, owned or used, that
// the registry of the package in the directory dir in s records, by name,
// with their paths.
func registryColumns(t *testing.T, s *sandbox, dir string) map[string][]registry.Path {
	t.Helper()
	g, err := registry.Parse(s.read(path.Join(dir, "testdata/hypercase.json")))
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
	return got
}

// checkPaths checks that the registry of each package of the topology, in
// s, records each column's paths as the chain gives them.
func (tp topology) checkPaths(t *testing.T, s *sandbox) {
	t.Helper()
	n := len(tp.kinds)
	for _, p := range tp.pkgs() {
		got := registryColumns(t, s, pkgDir(p))
		if len(got) == 0 {
			t.Errorf("%s's registry records no column", pkgDir(p))
		}
		for j := 1; j <= n+1; j++ {
			col := fmt.Sprintf("l%d", j)
			switch {
			case j == n+1 && p == 'c':
				col = "listc"
			case j == n+1:
				col = "list"
			case j == tp.shadow && tp.owner(p, j) == 'b':
				col += "x"
			}
			var want []registry.Path
			for _, c := range tp.corpora(p, j) {
				want = append(want, registry.Path{Corpus: fmt.Sprintf("top%d", c), Lifts: tp.lifts(p, j, c)})
			}
			if ps, ok := got[col]; ok && !reflect.DeepEqual(ps, want) {
				t.Errorf("%s's registry records %s's paths as %+v, and the chain gives %+v", pkgDir(p), col, ps, want)
			}
		}
	}
}

// A branching is a topology of two branches from Top0, where topologies
// gives chains alone: the bytes lifts l1 and l2 take Top0 and make Top1B
// and Top2B, the stages l3 and l4 take those, and the suites list3 and
// list4 list what l3 and l4 make. owners gives the owner, a or b, of the
// columns of branchCols, in their order, and late, for a and then b, that
// the package starts from Top1B and Top2B, and not from Top0. A package
// registers the columns it owns, the lifts before them on their branch,
// from its start, that it doesn't own, with hypercase.Use, and the corpus
// type of each branch it registers a column of.
type branching struct {
	owners string
	late   [2]bool
}

// branchCols are a branching's columns: l1 and l2, then l3 and l4, then
// list3 and list4. The column at index i is on branch i%2+1, at depth
// i/2.
var branchCols = []string{"l1", "l2", "l3", "l4", "list3", "list4"}

// branchings are every branching in which a and b each own a column, and
// a package that starts late owns neither bytes lift: one starts from
// Top0. A package uses a lift from a corpus its owner doesn't run it over
// where it starts early and the owner late, and two such uses on two
// branches are two failures; every package reaches a column from a corpus
// by one path, so none uses a lift by another.
func branchings() []branching {
	var out []branching
	for mask := range 1 << len(branchCols) {
		var ob strings.Builder
		for i := range branchCols {
			ob.WriteByte("ab"[mask>>i&1])
		}
		owners := ob.String()
		if !strings.Contains(owners, "a") || !strings.Contains(owners, "b") {
			continue
		}
		for _, late := range [][2]bool{{false, false}, {false, true}, {true, false}} {
			br := branching{owners, late}
			if (!late[0] || !strings.ContainsRune(owners[:2], 'a')) && (!late[1] || !strings.ContainsRune(owners[:2], 'b')) {
				out = append(out, br)
			}
		}
	}
	return out
}

func (br branching) name() string {
	late := []byte("ee")
	for i, l := range br.late {
		if l {
			late[i] = 'l'
		}
	}
	return "branching-" + br.owners + "-" + string(late)
}

// registration names the registration of the package pkg, a or b, which
// topologyRegistration reads.
func (br branching) registration(pkg byte) string { return br.name() + "-" + string(pkg) }

// columns are the indices in branchCols of the columns the package pkg
// owns, and of the lifts it uses.
func (br branching) columns(pkg byte) (owned, used []int) {
	for i := range branchCols {
		if br.owners[i] == pkg {
			owned = append(owned, i)
		}
	}
	for i := range 4 {
		if br.owners[i] == pkg || (br.late[pkg-'a'] && i < 2) {
			continue
		}
		if slices.ContainsFunc(owned, func(j int) bool { return j%2 == i%2 && j > i }) {
			used = append(used, i)
		}
	}
	return owned, used
}

// paths are the paths by which the package pkg reaches the column at
// index i in branchCols, as a registry records them: from Top0, where the
// package starts there, through the lifts before the column on its
// branch, and from the product of the branch's bytes lift, past it,
// through the stage, for a suite.
func (br branching) paths(pkg byte, i int) []registry.Path {
	branch, depth := i%2+1, i/2
	stage, bytes := fmt.Sprintf("l%d", branch+2), fmt.Sprintf("l%d", branch)
	var out []registry.Path
	if !br.late[pkg-'a'] {
		lifts := []string{}
		switch depth {
		case 1:
			lifts = []string{bytes}
		case 2:
			lifts = []string{stage, bytes}
		}
		out = append(out, registry.Path{Corpus: "top0", Lifts: lifts})
	}
	if depth > 0 {
		lifts := []string{}
		if depth == 2 {
			lifts = []string{stage}
		}
		out = append(out, registry.Path{Corpus: fmt.Sprintf("top%d", branch), Lifts: lifts})
	}
	return out
}

// column is the branching's column at index i in branchCols.
func (br branching) column(i int) any {
	product := func(k int) []decl.Output {
		return []decl.Output{
			{Pattern: fmt.Sprintf("l%d/*", k), Description: fmt.Sprintf("What l%d does, as its tree says.", k)},
			{Pattern: "data/*", Description: "The data a list lists."},
		}
	}
	switch i {
	case 0:
		return topologyBytesOf[Top0, Top1B](1, product(3))
	case 1:
		return topologyBytesOf[Top0, Top2B](2, product(4))
	case 2:
		return topologyStage[Top1B](3, func(v fs.FS) *Top3N { return &Top3N{v} })
	case 3:
		return topologyStage[Top2B](4, func(v fs.FS) *Top4N { return &Top4N{v} })
	case 4:
		return renamed(topologySuite[*Top3N](), "list3")
	}
	return renamed(topologySuite[*Top4N](), "list4")
}

// register registers the package pkg, a or b, of the branching.
func (br branching) register(pkg byte) {
	owned, used := br.columns(pkg)
	items := []any{project}
	if !br.late[pkg-'a'] {
		items = append(items, topologyTop0)
	}
	for b := range 2 {
		if slices.ContainsFunc(append(slices.Clone(owned), used...), func(i int) bool { return i%2 == b }) {
			items = append(items, topologyType(b+1))
		}
	}
	if pkg == 'a' {
		items = append(items, rootPkg)
	}
	for k := 1; k <= 4; k++ {
		items = append(items, topologyProperty(k))
	}
	var ownedCols, usedCols []any
	for _, i := range owned {
		ownedCols = append(ownedCols, br.column(i))
	}
	for _, i := range used {
		usedCols = append(usedCols, br.column(i))
	}
	items = append(items, own(ownedCols...))
	if len(usedCols) > 0 {
		items = append(items, hypercase.Use(usedCols...))
	}
	hypercase.Register(items...)
}

// sandbox is a sandbox holding the branching's packages, a base archive
// at Top0, whose data sits below each bytes lift's out/ tree, and one at
// the product of each branch a late package registers, after the first
// update, and the grid of its packages.
func (br branching) sandbox(t *testing.T) (*sandbox, closingGrid) {
	t.Helper()
	s := newSandbox(t, "")
	s.write("b/corpus_test.go", []byte(packageTestFile), "./b is a package with tests")
	g := closingGrid{pkgs: []childRun{{pkg: "", registration: br.registration('a')}, {pkg: "b", registration: br.registration('b')}}}
	s.write("testdata/top0/base.txtar", []byte("Two branches of lifts, each following its tree.\n-- in/l1/out/data/A1 --\n1\n-- in/l2/out/data/B1 --\n2\n"), "the branching's base at Top0")
	for b := 1; b <= 2; b++ {
		for _, p := range []byte("ab") {
			owned, used := br.columns(p)
			if br.late[p-'a'] && slices.ContainsFunc(append(owned, used...), func(i int) bool { return i%2+1 == b }) {
				s.write(fmt.Sprintf("testdata/top%d/base.txtar", b), fmt.Appendf(nil, "A branch's data.\n-- in/data/A%d --\n%d\n", b, b), "the branching's base where a package starts late")
				break
			}
		}
	}
	updateAll(t, s, g)
	return s, g
}

// refusals are the root-package check's failures over the branching,
// from its description alone: one for each package that uses a lift, and
// each corpus it reaches the lift from that the lift's owner doesn't
// reach it from alike.
func (br branching) refusals() []usedRefusal {
	var out []usedRefusal
	for _, u := range []byte("ab") {
		_, used := br.columns(u)
		for _, i := range used {
			o, lift := br.owners[i], branchCols[i]
			theirs := br.paths(o, i)
			for _, p := range br.paths(u, i) {
				r := usedRefusal{pkgDir(u), pkgDir(o), p.Corpus, lift, usedRoute(p.Corpus, p.Lifts, lift), ""}
				switch j := slices.IndexFunc(theirs, func(q registry.Path) bool { return q.Corpus == p.Corpus }); {
				case j < 0:
				case !slices.Equal(p.Lifts, theirs[j].Lifts):
					r.theirs = usedRoute(p.Corpus, theirs[j].Lifts, lift)
				default:
					continue
				}
				out = append(out, r)
			}
		}
	}
	return out
}

// checkPaths checks that the registry of each package of the branching,
// in s, records the columns it owns and the lifts it uses, and each one's
// paths as paths gives them.
func (br branching) checkPaths(t *testing.T, s *sandbox) {
	t.Helper()
	for _, p := range []byte("ab") {
		got := registryColumns(t, s, pkgDir(p))
		owned, used := br.columns(p)
		want := map[string][]registry.Path{}
		for _, i := range append(owned, used...) {
			want[branchCols[i]] = br.paths(p, i)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s's registry records the columns and paths\n%+v\nand the branching gives\n%+v", pkgDir(p), got, want)
		}
	}
}
