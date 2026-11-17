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
	"sync"
	"testing"
)

// A source is the package's non-test source, type-checked.
type source struct {
	fset  *token.FileSet
	files []*ast.File
	pkg   *types.Package
	info  *types.Info
}

var (
	loaded     *source
	loadErr    error
	loadedOnce sync.Once
)

// loadSource type-checks the package's non-test source, as go list gives
// its files, against its imports' export data, which go list builds. It
// uses the go command and the standard library alone, so the module needs
// no loader: golang.org/x/tools/go/packages would bring in
// golang.org/x/mod, a module hypercase doesn't otherwise require.
func loadSource(t *testing.T) *source {
	t.Helper()
	loadedOnce.Do(func() { loaded, loadErr = typeCheck() })
	if loadErr != nil {
		t.Fatalf("type-checking the package: %v", loadErr)
	}
	return loaded
}

func typeCheck() (*source, error) {
	cmd := exec.Command("go", "list", "-export", "-deps", "-json=ImportPath,Dir,GoFiles,Export,DepOnly", ".")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %v\n%s", err, stderr.Bytes())
	}
	type listed struct {
		ImportPath, Dir, Export string
		GoFiles                 []string
		DepOnly                 bool
	}
	exports := map[string]string{}
	var self *listed
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p listed
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		exports[p.ImportPath] = p.Export
		if !p.DepOnly {
			self = &p
		}
	}
	if self == nil {
		return nil, errors.New("go list gave no package for .")
	}
	s := &source{fset: token.NewFileSet()}
	for _, name := range self.GoFiles {
		f, err := parser.ParseFile(s.fset, filepath.Join(self.Dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		s.files = append(s.files, f)
	}
	imp := importer.ForCompiler(s.fset, "gc", func(path string) (io.ReadCloser, error) {
		if exports[path] == "" {
			return nil, fmt.Errorf("go list gave no export data for %s", path)
		}
		return os.Open(exports[path])
	})
	s.info = &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	conf := types.Config{Importer: imp}
	s.pkg, err = conf.Check(self.ImportPath, s.fset, s.files, s.info)
	if err != nil {
		return nil, err
	}
	return s, nil
}

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
