package hypercase

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// A source is the package's non-test source, type-checked. deps are the
// files of each package of its module that it imports, directly or not,
// which the type checker reads from source too, so that an object the
// package names from one of them is the one that package declares:
// depPkgs holds those packages.
type source struct {
	fset    *token.FileSet
	files   []*ast.File
	pkg     *types.Package
	info    *types.Info
	deps    []*ast.File
	depPkgs map[*types.Package]bool
}

var (
	loaded     *source
	loadErr    error
	loadedOnce sync.Once
)

// loadSource type-checks the package's non-test source, as go list gives
// its files, and that of every package of its module that it imports,
// against the export data of the other modules' packages and the standard
// library's, which go list builds. It uses the go command and the
// standard library alone, so the module needs no loader. It fails on a
// non-test file of one of the module's packages that build constraints
// exclude here, which it can't read, so no check of the source misses one.
func loadSource(t *testing.T) *source {
	t.Helper()
	loadedOnce.Do(func() { loaded, loadErr = typeCheck(".") })
	if loadErr != nil {
		t.Fatalf("type-checking the package: %v", loadErr)
	}
	return loaded
}

// An importerFunc imports a package by its path.
type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }

// typeCheck type-checks the non-test source of the package in dir, as
// loadSource does.
func typeCheck(dir string) (*source, error) {
	cmd := exec.Command("go", "list", "-export", "-deps", "-json=ImportPath,Dir,GoFiles,IgnoredGoFiles,Export,DepOnly,Module", ".")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %v\n%s", err, stderr.Bytes())
	}
	type listed struct {
		ImportPath, Dir, Export string
		GoFiles, IgnoredGoFiles []string
		DepOnly                 bool
		Module                  *struct{ Main bool }
	}
	pkgs := map[string]*listed{}
	var self *listed
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p listed
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		pkgs[p.ImportPath] = &p
		if !p.DepOnly {
			self = &p
		}
	}
	if self == nil {
		return nil, errors.New("go list gave no package for .")
	}
	s := &source{fset: token.NewFileSet(), depPkgs: map[*types.Package]bool{}}
	s.info = &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	// parse parses a package's non-test files. The check reads the
	// non-test source alone, so a test file the build excludes misses
	// nothing.
	parse := func(p *listed) ([]*ast.File, error) {
		var ignored []string
		for _, name := range p.IgnoredGoFiles {
			if !strings.HasSuffix(name, "_test.go") {
				ignored = append(ignored, name)
			}
		}
		if len(ignored) > 0 {
			return nil, fmt.Errorf("build constraints exclude %v from %s on this platform, so the type check can't read them, and a check of the source would miss what they hold. Keep every file of the package in every build, or check it on each platform", ignored, p.ImportPath)
		}
		var files []*ast.File
		for _, name := range p.GoFiles {
			f, err := parser.ParseFile(s.fset, filepath.Join(p.Dir, name), nil, parser.SkipObjectResolution)
			if err != nil {
				return nil, err
			}
			files = append(files, f)
		}
		return files, nil
	}
	exports := importer.ForCompiler(s.fset, "gc", func(path string) (io.ReadCloser, error) {
		if p := pkgs[path]; p != nil && p.Export != "" {
			return os.Open(p.Export)
		}
		return nil, fmt.Errorf("go list gave no export data for %s", path)
	})
	// A package of the module is read from source, once.
	checked := map[string]*types.Package{}
	var imp importerFunc
	imp = func(path string) (*types.Package, error) {
		if pkg, ok := checked[path]; ok {
			return pkg, nil
		}
		p := pkgs[path]
		if p == nil || p.Module == nil || !p.Module.Main {
			return exports.Import(path)
		}
		files, err := parse(p)
		if err != nil {
			return nil, err
		}
		pkg, err := (&types.Config{Importer: imp}).Check(path, s.fset, files, s.info)
		if err != nil {
			return nil, err
		}
		checked[path] = pkg
		s.deps = append(s.deps, files...)
		s.depPkgs[pkg] = true
		return pkg, nil
	}
	if s.files, err = parse(self); err != nil {
		return nil, err
	}
	conf := types.Config{Importer: imp}
	s.pkg, err = conf.Check(self.ImportPath, s.fset, s.files, s.info)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// ours reports whether a package is the one source checks, or one of its
// module that it imports, whose source it reads.
func (s *source) ours(pkg *types.Package) bool { return pkg != nil && (pkg == s.pkg || s.depPkgs[pkg]) }

// funcName names a function declaration as its messages are listed: a
// method by its receiver's type name, and a dot, whatever the type's
// parameters.
func (s *source) funcName(fd *ast.FuncDecl) string {
	fn := s.info.Defs[fd.Name].(*types.Func)
	recv := fn.Type().(*types.Signature).Recv()
	if recv == nil {
		return fn.Name()
	}
	typ := recv.Type()
	if p, ok := typ.(*types.Pointer); ok {
		typ = p.Elem()
	}
	return typ.(*types.Named).Obj().Name() + "." + fn.Name()
}

// constants returns the package's constants of a named integer type, by
// value.
func (s *source) constants(typeName string) map[int64]string {
	named := s.pkg.Scope().Lookup(typeName).Type()
	out := map[int64]string{}
	for _, name := range s.pkg.Scope().Names() {
		c, ok := s.pkg.Scope().Lookup(name).(*types.Const)
		if !ok || !types.Identical(c.Type(), named) {
			continue
		}
		v, ok := constant.Int64Val(c.Val())
		if !ok {
			panic(fmt.Sprintf("the constant %s, of %s, is no int64", name, typeName))
		}
		out[v] = c.Name()
	}
	return out
}

// funcDecl returns the declaration of a function, named as funcName names
// it.
func (s *source) funcDecl(t *testing.T, name string) *ast.FuncDecl {
	t.Helper()
	for _, f := range s.files {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && s.funcName(fd) == name {
				return fd
			}
		}
	}
	t.Fatalf("the package declares no function %s", name)
	return nil
}

// stringsUsedIn returns the values of the package's string constants that
// a function's body names, in the order it first names them.
func (s *source) stringsUsedIn(t *testing.T, name string) []string {
	t.Helper()
	var out []string
	ast.Inspect(s.funcDecl(t, name).Body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		c, ok := s.info.Uses[id].(*types.Const)
		if !ok || c.Pkg() != s.pkg || c.Parent() != s.pkg.Scope() || c.Val().Kind() != constant.String {
			return true
		}
		if v := constant.StringVal(c.Val()); !slices.Contains(out, v) {
			out = append(out, v)
		}
		return true
	})
	return out
}

// TestTypeCheckIgnored: the type check reads a package's non-test source,
// so it refuses a non-test file that build constraints exclude, which it
// can't read, and type-checks a package whose excluded files are tests.
func TestTypeCheckIgnored(t *testing.T) {
	t.Parallel()
	excluded := "//go:build never\n\npackage p\n"
	dir := tempPackage(t, map[string]string{"p.go": "package p\n", "q_test.go": excluded})
	if _, err := typeCheck(dir); err != nil {
		t.Errorf("type-checking a package whose one excluded file is a test: %v", err)
	}
	dir = tempPackage(t, map[string]string{"p.go": "package p\n", "q.go": excluded, "r_test.go": excluded})
	_, err := typeCheck(dir)
	if err == nil || !strings.Contains(err.Error(), "exclude [q.go] from example.com/p") {
		t.Errorf("type-checking a package with an excluded non-test file, q.go, beside an excluded test, gives %v", err)
	}
}

// tempPackage writes a module of one package, example.com/p, with the
// files given, by name, and returns its directory.
func tempPackage(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files["go.mod"] = "module example.com/p\n\ngo 1.22\n"
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
