package snapshot

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/hooks"
)

// files returns every file of a tree, by path.
func files(t *testing.T, tree fs.FS) map[string]string {
	t.Helper()
	paths, err := archive.Paths(tree)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, p := range paths {
		data, err := fs.ReadFile(tree, p)
		if err != nil {
			t.Fatal(err)
		}
		m[p] = string(data)
	}
	return m
}

func sameFiles(t *testing.T, what string, got, want map[string]string) {
	t.Helper()
	if !maps.Equal(got, want) {
		t.Errorf("%s holds:\n%q\nnot:\n%q", what, got, want)
	}
}

// TestLoad: Load returns a value whose fs.FS holds exactly the serialized
// files and bytes, the trees as its top-level directories.
func TestLoad(t *testing.T) {
	path, err := Create(newCorpus(t), project, sheetType, "twice", serialize(t, cells))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Load(sheetType, path)
	if err != nil {
		t.Fatal(err)
	}
	sameFiles(t, "the loaded case", files(t, s), map[string]string{"sheet/A1": "=B1*2\n", "sheet/B1": "3\n"})
}

// TestLoadRefuses: a hand-written archive with an in/params/ tree, or with
// a top-level name outside the closed set; one that isn't in the type's
// directory; one whose name breaks the naming rule, or isn't an archive's;
// one outside testdata/<dir>/; one that can't be read; and an empty path:
// Load refuses each, naming the path.
func TestLoadRefuses(t *testing.T) {
	root := t.TempDir()
	at := func(rel, data string) string {
		p := filepath.Join(root, filepath.FromSlash(rel))
		writeFile(t, p, []byte(data))
		return p
	}
	for _, c := range []struct {
		path string
		want []string
	}{
		{at("testdata/sheet/params.txtar", "A case with parameters.\n-- in/sheet/A1 --\n1\n-- in/params/x --\n2\n"), []string{"has problems of form", "in/params/x: the tree name params is reserved"}},
		{at("testdata/sheet/typo.txtar", "A typo.\n-- ni/sheet/A1 --\n1\n"), []string{"has problems of form", "ni/sheet/A1: ni is not one of the framework's top-level names"}},
		{at("testdata/csv/prices.txtar", "A CSV.\n-- in/csv/prices.csv --\n4,6\n"), []string{"is in testdata/csv/, and the corpus type's directory is sheet"}},
		{at("testdata/sheet/Twice.txtar", "A1.\n-- in/sheet/A1 --\n1\n"), []string{`the archive's name, "Twice", breaks the naming rule`}},
		{at("testdata/sheet/twice.txt", "A1.\n-- in/sheet/A1 --\n1\n"), []string{"isn't an archive: an archive's file name ends in .txtar"}},
		{at("sheet/twice.txtar", "A1.\n-- in/sheet/A1 --\n1\n"), []string{"isn't in a corpus's directory, testdata/<dir>/"}},
		{filepath.Join(root, "testdata", "sheet", "gone.txtar"), []string{"reading " + filepath.Join(root, "testdata", "sheet", "gone.txtar")}},
	} {
		_, err := Load(sheetType, c.path)
		fails(t, err, append([]string{c.path}, c.want...)...)
	}
	_, err := Load(decl.Type[Sheet]{Dir: "checks"}, at("testdata/checks/x.txtar", "X.\n-- in/sheet/A1 --\n1\n"))
	fails(t, err, "the corpus type's directory, checks, is reserved")
	_, err = Load(sheetType, "")
	fails(t, err, "the archive's path is empty.\nPass the path of the case's archive.")
}

// TestExtractBase: Extract of the base writes exactly the files and bytes
// Load's fs.FS holds, under <dir>/<tree>/.
func TestExtractBase(t *testing.T) {
	_, path := corpus(t)
	s, err := Load(sheetType, path)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "twice")
	if err := Extract(path, "", dir); err != nil {
		t.Fatal(err)
	}
	want := files(t, s)
	sameFiles(t, dir, files(t, os.DirFS(dir)), want)
	sameFiles(t, "the base", want, map[string]string{"sheet/A1": "=B1*2\n", "sheet/B1": "3\n", "sheet/D1": "=Z9\n"})
}

// TestExtractVariant: Extract of cycle writes the base with
// variant/cycle/'s overlay and deletions applied: the replaced B1 holds
// the overlay's bytes, the added C1 is there, and the deleted D1 isn't.
func TestExtractVariant(t *testing.T) {
	_, path := corpus(t)
	dir := filepath.Join(t.TempDir(), "cycle")
	if err := Extract(path, "cycle", dir); err != nil {
		t.Fatal(err)
	}
	sameFiles(t, dir, files(t, os.DirFS(dir)), map[string]string{"sheet/A1": "=B1*2\n", "sheet/B1": "=A1\n", "sheet/C1": "=B1\n"})
}

// TestExtractRefuses: Extract to an existing dir fails, naming it, and
// leaves it as it was. Extract of a variant the archive lacks fails,
// naming it, and creates no dir; so does Extract of looped, which only a
// transform's variant is, and the message says a transform's variant
// isn't extracted. So does Extract to a dir whose parent is missing, of an
// archive Load refuses, and of an empty path, named as such.
func TestExtractRefuses(t *testing.T) {
	_, path := corpus(t)
	dir := filepath.Join(t.TempDir(), "x")
	writeFile(t, filepath.Join(dir, "mine"), []byte("a person's file\n"))
	fails(t, Extract(path, "", dir), dir+" exists, and Extract writes a directory of its own")
	sameFiles(t, dir, files(t, os.DirFS(dir)), map[string]string{"mine": "a person's file\n"})

	for _, c := range []struct {
		variant string
		want    string
	}{
		{"gone", path + " has no variant/gone/.\nName one of its hand-written variants: cycle, missing."},
		{"looped", path + " has no variant/looped/, only transform/looped/, and a transform's variant isn't extracted"},
	} {
		dir := filepath.Join(t.TempDir(), "x")
		fails(t, Extract(path, c.variant, dir), c.want)
		absent(t, dir)
	}
	dir = filepath.Join(t.TempDir(), "no", "x")
	fails(t, Extract(path, "", dir), "creating "+dir)
	absent(t, dir)

	root := t.TempDir()
	bad := filepath.Join(root, "testdata", "sheet", "typo.txtar")
	writeFile(t, bad, []byte("A typo.\n-- ni/sheet/A1 --\n1\n"))
	dir = filepath.Join(t.TempDir(), "x")
	fails(t, Extract(bad, "", dir), bad+" has problems of form")
	absent(t, dir)
	fails(t, Extract("", "", dir), "the archive's path is empty.\nPass the path of the case's archive.")
	absent(t, dir)
}

// TestExtractNoVariants: Extract of a variant of an archive with no
// hand-written variant says so.
func TestExtractNoVariants(t *testing.T) {
	path, err := Create(newCorpus(t), project, sheetType, "twice", serialize(t, cells))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "x")
	fails(t, Extract(path, "cycle", dir), path+" has no variant/cycle/, and no hand-written variant at all")
	absent(t, dir)
}

// TestExtractUndefinedVariant: Extract of a variant whose delete names a
// path the base lacks fails, naming the variant, and creates no dir.
func TestExtractUndefinedVariant(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "testdata", "sheet", "twice.txtar")
	writeFile(t, path, []byte("A1 is twice B1.\n-- in/sheet/A1 --\n=B1*2\n-- in/sheet/B1 --\n3\n-- variant/gone/about --\nQ1 is gone.\n-- variant/gone/delete --\nsheet/Q1\n"))
	dir := filepath.Join(t.TempDir(), "x")
	fails(t, Extract(path, "gone", dir), path+": the variant gone defines no case", "variant/gone/delete, line 1: the base has no sheet/Q1 to remove")
	absent(t, dir)
}

// TestExtractFailsPartWay: a write made to fail part-way leaves no dir.
func TestExtractFailsPartWay(t *testing.T) {
	_, path := corpus(t)
	dir := filepath.Join(t.TempDir(), "x")
	var wrote []string
	hooks.ExtractFile = func(p string) error {
		wrote = append(wrote, p)
		if len(wrote) == 2 {
			return errors.New("the disk is full")
		}
		return nil
	}
	err := Extract(path, "", dir)
	hooks.ExtractFile = nil
	fails(t, err, "writing "+filepath.Join(dir, "sheet", "B1")+": the disk is full, so "+dir+" is removed.\nFix what stopped the write")
	absent(t, dir)
}

// TestExtractRemoveFails: a write that fails part-way, after which Extract
// can't remove dir, here because the hook made dir's parent read-only,
// fails naming both failures, and says to remove the directory.
func TestExtractRemoveFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root removes from a read-only directory")
	}
	_, path := corpus(t)
	parent := t.TempDir()
	t.Cleanup(func() { os.Chmod(parent, 0o755) })
	dir := filepath.Join(parent, "x")
	hooks.ExtractFile = func(p string) error {
		if err := os.Chmod(parent, 0o555); err != nil {
			return err
		}
		return errors.New("the disk is full")
	}
	err := Extract(path, "", dir)
	hooks.ExtractFile = nil
	fails(t, err, "writing "+filepath.Join(dir, "sheet", "A1")+": the disk is full, and removing "+dir+": unlinkat "+dir+": permission denied.\nRemove the directory. Fix what stopped the write, and extract the case again.")
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("Extract removed %s, though its parent is read-only: %v", dir, err)
	}
}

// TestExtractReadsBack: files that don't read back as the view, as on a
// filesystem that folds names, fail Extract, and leave no dir.
func TestExtractReadsBack(t *testing.T) {
	_, path := corpus(t)
	for _, c := range []struct {
		change func(p string) error
		want   string
	}{
		{func(p string) error { return os.WriteFile(p, []byte("changed\n"), 0o644) }, "holds other bytes than the view's sheet/A1"},
		{func(p string) error { return os.WriteFile(filepath.Join(filepath.Dir(p), "Z1"), nil, 0o644) }, "the files under"},
	} {
		dir := filepath.Join(t.TempDir(), "x")
		hooks.ExtractFile = func(p string) error {
			if filepath.Base(p) == "A1" {
				return c.change(p)
			}
			return nil
		}
		err := Extract(path, "", dir)
		hooks.ExtractFile = nil
		fails(t, err, "doesn't read back as the case's input view", c.want, dir+" is removed.\nExtract to a filesystem that keeps each name and each byte as written.")
		absent(t, dir)
	}
}
