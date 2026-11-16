package hypercase_test

import (
	"bytes"
	"errors"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"github.com/sdboyer/hypercase"
	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/jenny"
)

// Step 7's declarations: transforms over Script.

// steeredTransform returns a transform over Script that copies the base's
// input view and applies change to it. Its probe says no with decline's
// reason, and the base's tr/ tree steers how it goes wrong:
//
//	tr/probe        the probe says no with the reason it holds, and an
//	                empty one with no reason
//	tr/probe-panic  the probe panics with what it holds
//	tr/error        the jenny fails with what it holds
//	tr/panic        the jenny panics with what it holds
//	tr/same         the jenny makes the base's view, unchanged
//	tr/flat         the jenny adds a file outside any tree
//	tr/marker       the jenny adds do/out/m.txt, with a line shaped like a
//	                section marker
//	tr/nest         the jenny adds tr/nest/x, below the file tr/nest
func steeredTransform(name, description string, change func(files map[string][]byte), decline func(in fs.FS) (string, bool)) decl.Transform[Script] {
	return decl.Transform[Script]{
		Name:        name,
		Description: description,
		Probe: func(base Script) (string, bool) {
			if why, ok := steeredProbe(base, "tr"); !ok {
				return why, false
			}
			if decline != nil {
				return decline(base)
			}
			return "", true
		},
		Jenny: jenny.For[Script]().New(name, func(base Script) (jenny.Files, error) {
			paths, err := archive.Paths(base)
			if err != nil {
				return nil, err
			}
			files := map[string][]byte{}
			for _, p := range paths {
				if files[p], err = fs.ReadFile(base, p); err != nil {
					return nil, err
				}
			}
			if data, ok := files["tr/error"]; ok {
				return nil, errors.New(strings.TrimSuffix(string(data), "\n"))
			}
			if data, ok := files["tr/panic"]; ok {
				panic(strings.TrimSuffix(string(data), "\n"))
			}
			if _, ok := files["tr/same"]; !ok {
				change(files)
			}
			if _, ok := files["tr/flat"]; ok {
				files["flat"] = []byte("x\n")
			}
			if _, ok := files["tr/marker"]; ok {
				files["do/out/m.txt"] = []byte("a\n-- x --\n")
			}
			if _, ok := files["tr/nest"]; ok {
				files["tr/nest/x"] = []byte("x\n")
			}
			var out jenny.Files
			for _, p := range slices.Sorted(maps.Keys(files)) {
				out = append(out, jenny.NewFile(p, files[p]))
			}
			return out, nil
		}),
	}
}

var (
	// looped makes echo and resolve err, as on a cycle, and gives circular.
	looped = func() decl.Transform[Script] {
		tr := steeredTransform("looped", "Adds do/error and resolve/error, so echo and resolve err as on a cycle.",
			func(files map[string][]byte) {
				files["do/error"] = []byte("A1 -> A1: cycle\n")
				files["resolve/error"] = []byte("A1 -> A1: cycle\n")
			},
			func(in fs.FS) (string, bool) {
				if _, err := fs.Stat(in, "do/error"); err == nil {
					return "the base's do/ tree already errs", false
				}
				return "", true
			})
		tr.Gives = []decl.Property{circular}
		return tr
	}()
	// upper changes what echo prints, and keeps circular and dangling.
	upper = func() decl.Transform[Script] {
		tr := steeredTransform("upper", "Uppercases every file under do/out/.",
			func(files map[string][]byte) {
				for p, data := range files {
					if strings.HasPrefix(p, "do/out/") {
						files[p] = bytes.ToUpper(data)
					}
				}
			},
			func(in fs.FS) (string, bool) {
				if matches, _ := fs.Glob(in, "do/out/*"); len(matches) == 0 {
					return "the base's do/ tree has no output", false
				}
				return "", true
			})
		tr.Keeps = []decl.Property{circular, dangling}
		return tr
	}()
	// poke adds a file, for the ways a transform goes wrong.
	poke = steeredTransform("poke", "Adds do/out/p.txt.", func(files map[string][]byte) { files["do/out/p.txt"] = []byte("poked\n") }, nil)
	// trim removes a file, so its variants have a delete section, and adds
	// one, so it makes a variant of any base.
	trim = func() decl.Transform[Script] {
		tr := steeredTransform("trim", "Removes do/out/values.txt, and adds do/out/k.txt.",
			func(files map[string][]byte) {
				delete(files, "do/out/values.txt")
				files["do/out/k.txt"] = []byte("kept\n")
			}, nil)
		tr.Keeps = []decl.Property{circular}
		return tr
	}()

	transformType = decl.Type[Script]{Dir: "script", Description: scriptType.Description, Transforms: []decl.Transform[Script]{upper, looped}}
	pokeType      = decl.Type[Script]{Dir: "script", Description: scriptType.Description, Transforms: []decl.Transform[Script]{poke}}
)

func withTransformName(tr decl.Transform[Script], name string) decl.Transform[Script] {
	tr.Name = name
	return tr
}

// typeWith is the script type with the given transforms.
func typeWith(trs ...decl.Transform[Script]) decl.Type[Script] {
	return decl.Type[Script]{Dir: "script", Description: scriptType.Description, Transforms: trs}
}

var sheetsProps = []any{circular, dangling, malformed}

func transforms(items ...any) {
	hypercase.Register(append([]any{project, rootPkg}, append(sheetsProps, items...)...)...)
}

var echoRejects = withRejects(echo, circular, dangling)

// transformRegistrations are step 7's declaration sets, by name.
var transformRegistrations = map[string]func(){
	"variants":   func() { transforms(scriptType, own(echoRejects)) },
	"variants-b": func() { hypercase.Register(append([]any{project, scriptType, own(shout)}, sheetsProps...)...) },
	"transforms": func() { transforms(transformType, own(echoRejects, looped, upper)) },
	"tr-ladder":  func() { transforms(transformType, own(parse, resolve, looped, upper)) },
	"tr-unowned": func() { transforms(transformType, own(echoRejects)) },
	"tr-b": func() {
		hypercase.Register(append([]any{project, transformType, own(withName(echoRejects, "copy"))}, sheetsProps...)...)
	},
	"tr-b-owns": func() {
		hypercase.Register(append([]any{project, transformType, own(withName(echoRejects, "copy"), upper)}, sheetsProps...)...)
	},
	"tr-b-differs": func() {
		hypercase.Register(append([]any{project, typeWith(looped), own(withName(echoRejects, "copy"))}, sheetsProps...)...)
	},
	"poke": func() { transforms(pokeType, own(echoRejects, poke)) },
	"trim": func() { transforms(typeWith(trim), own(echoRejects, trim)) },
	"poke-b": func() {
		hypercase.Register(append([]any{project, pokeType, own(withName(echoRejects, "copy"))}, sheetsProps...)...)
	},

	// Refusals at registration.
	"tr-bare":        func() { transforms(transformType, looped) },
	"tr-use":         func() { transforms(transformType, hypercase.Use(looped)) },
	"tr-own-twice":   func() { transforms(transformType, own(looped, looped)) },
	"tr-bad-name":    func() { transforms(typeWith(withTransformName(looped, "Looped"))) },
	"tr-two-names":   func() { transforms(typeWith(looped, withTransformName(upper, "looped"))) },
	"tr-no-jenny":    func() { tr := looped; tr.Jenny = jenny.Jenny[Script]{}; transforms(typeWith(tr)) },
	"tr-gives-keeps": func() { tr := looped; tr.Keeps = []decl.Property{circular}; transforms(typeWith(tr)) },
	"tr-gives-twice": func() { tr := looped; tr.Gives = []decl.Property{circular, circular}; transforms(typeWith(tr)) },
	"tr-keeps-twice": func() { tr := upper; tr.Keeps = []decl.Property{dangling, dangling}; transforms(typeWith(tr)) },
	"col-variant":    func() { transforms(scriptType, own(withName(echo, "variant"))) },
	"col-transform":  func() { transforms(scriptType, own(withName(echo, "transform"))) },
	"lift-variant":   func() { l := parse; l.Name = "variant"; transforms(scriptType, own(l)) },

	// Refusals once initialization is over.
	"tr-own-unlisted": func() { transforms(typeWith(looped), own(echoRejects, looped, upper)) },
	"tr-own-no-type":  func() { transforms(otherType, own(steered[Other]("echo", "do"), looped)) },
	"tr-own-differs": func() {
		tr := looped
		tr.Description = "Another description."
		transforms(transformType, own(echoRejects, tr, upper))
	},
	"tr-unregistered-gives": func() {
		hypercase.Register(project, rootPkg, dangling, typeWith(looped), own(withRejects(echo, dangling), looped))
	},
	"tr-other-description": func() {
		hypercase.Register(project, rootPkg, loop, dangling, typeWith(upper), own(withRejects(echo, loop), upper))
	},
	"tr-no-description": func() {
		tr := looped
		tr.Description = " "
		transforms(typeWith(tr), own(echoRejects, tr))
	},
}
