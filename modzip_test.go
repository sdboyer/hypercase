package hypercase_test

import (
	"errors"
	"testing"

	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"
)

// TestModuleZip: every file of the module is one a module zip can hold, so
// a consumer can require the module at any commit. zip.CheckDir applies
// the go command's own rule to the files under the module root, which is
// the root package's directory: each element of a file's path is one
// module.CheckFilePath admits, no two paths differ only in case, and the
// files fit the size limits. A file it refuses fails the module's download,
// so no consumer can require that commit.
func TestModuleZip(t *testing.T) {
	t.Parallel()
	cf, err := zip.CheckDir(".")
	for _, e := range cf.Invalid {
		t.Errorf("%s can't be in a module zip: %v.\nRename it. A transcript is named from its test: rename the test, and the transcript with it.", e.Path, ruleError(e.Err))
	}
	if cf.SizeError != nil {
		t.Errorf("the module's files can't be a module zip: %v.", cf.SizeError)
	}
	if err != nil && len(cf.Invalid) == 0 && cf.SizeError == nil {
		t.Fatalf("checking the module's files: %v", err)
	}
}

// ruleError is what of a path the module-zip rule refuses: the reason
// module.CheckFilePath gives, without the path it repeats.
func ruleError(err error) error {
	var p *module.InvalidPathError
	if errors.As(err, &p) {
		return p.Err
	}
	return err
}
