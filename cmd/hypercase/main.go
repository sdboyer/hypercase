// Command hypercase is hypercase's command line.
//
// Usage:
//
//	hypercase report [-root dir] [-text] <a> <b>
//	hypercase strip [-root dir] [-column name] [-transform name] [-frozen]
//
// report compares two git revisions of a corpus root, read from the
// repository's object store, never the working tree. A cell exists at a
// revision when its outcome file does. The report writes one HTML page on
// stdout, which runs no script and loads nothing: the problems it found
// and doesn't pick between; the cells that changed, ranked, with changed
// outcomes first, then cells that appeared, then cells that disappeared,
// then files that changed under the same outcome, each file with its line
// diff; the matrix of each corpus at <b>; and the descriptions of what the
// registries and archives declare, from <b>, or from <a> for what <b>
// lacks. With -text it writes the problems and the ranked changes as plain
// text instead.
//
// It reads registries of every format from the oldest this hypercase
// knows to its own, and refuses a newer one, naming the hypercase that
// reads it.
//
// report's -root names the corpus root, relative to the current directory
// and inside the repository; it refuses an absolute path. Without -root,
// the corpus root is the one the registry of the package in the current
// directory records, at <b>, or at <a> where <b> has no module there, or,
// with no registry there, the module root, from any directory of the
// module.
//
// strip removes update-written sections from the archives of a corpus root,
// telling them from hand-written ones by name alone, and leaves every
// hand-written section's bytes as they were. A person runs it, never an
// update; the update that follows writes the sections again from the inputs
// and the code. The archives are the *.txtar files directly in the
// directories directly under the corpus root's testdata/. Nothing under
// testdata/ that strip reaches, testdata/ itself included, is a symbolic
// link: a run reads through one, where strip, the root-package check and
// the report don't, so strip refuses one. It doesn't reach into a nested
// module or another corpus root. With no flags it strips every archive, and
// removes the registries that record the corpus root and the findings files
// in their packages' testdata/checks/. It keeps a package's registry while
// a findings file of the package is left, so that strip run again finds it,
// and every registry while it leaves a *.txtar file as it was, for a
// problem of form, a failed write or a place that is no archive's, so that
// strip run again reads the registries as this run did.
// -column limits it to a column's sections, at every level, and -transform
// to those under a transform's variant, transform/<name>/; given together,
// they limit it to the column's sections under that variant. Either leaves
// the registries and findings files as they are. It leaves a frozen cell's
// sections in place, and lists the cell; -frozen strips them too. It
// prints a line for each archive it changed, each frozen cell it left and
// each file it left with a problem, then the update to run. A frozen
// cell's line gives the command that strips it: -column with its column,
// -transform for a cell of a transform's variant, and -frozen. No flag
// limits strip to one archive or to a hand-written variant, so that
// command strips the column's other frozen cells too. A line that says to
// run strip again gives the command, with -root unless strip ran at the
// module root on the module-root corpus and the registry there, if there
// is one, records that corpus. A command quotes the root's path where a
// shell would read it as other than itself.
//
// strip exits 1 having removed nothing when it can't find the corpus root
// or the module root, or the corpus root holds no testdata/; when a
// registry under the module can't be read, or the registries that record
// the corpus root record different update variables; when reading the
// corpus root's testdata/ fails, or testdata/, or anything strip reaches
// under it, is a symbolic link; and when no archive that strip strips holds
// a section -column or -transform selects, counting a frozen cell's
// sections, which it leaves. It exits 1 too when it left an archive or a
// file in place for a problem, a failed write or removal included. It exits
// 2 for a usage error: a flag given twice, an unknown flag, an argument, an
// empty -root, or an empty or ill-formed -column or -transform.
//
// strip's -root names the corpus root, relative to the current directory,
// or by an absolute path. Without -root, the corpus root is the one the
// registry of the package in the current directory records, or, at the
// module root with no registry there, the module root. Anywhere else with
// no registry, strip asks for -root, and so it does for a registry's
// corpus root that lies inside a module nested in the package's.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/sdboyer/hypercase/internal/report"
)

func main() {
	os.Exit(run(".", nil, os.Args[1:], os.Stdout, os.Stderr))
}

const (
	reportUsage = `hypercase report [-root dir] [-text] <a> <b>`
	stripUsage  = `hypercase strip [-root dir] [-column name] [-transform name] [-frozen]`
	usage       = "usage: " + reportUsage + "\n       " + stripUsage
)

// run runs the command in dir, with git's environment env, or this
// process's when env is nil, and returns its exit code: 2 for a usage
// error, 1 for a failure.
func run(dir string, env []string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "report":
		return runReport(dir, env, args[1:], stdout, stderr)
	case "strip":
		return runStrip(dir, args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "hypercase: no command %q\n%s\n", args[0], usage)
	return 2
}

func runReport(dir string, env []string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: "+reportUsage)
		fs.PrintDefaults()
	}
	root := fs.String("root", "", "the corpus root, relative to the current directory")
	text := fs.Bool("text", false, "write the ranked changes as plain text instead of the page")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintf(stderr, "hypercase report takes two revisions, and was given %d.\nusage: %s\n", fs.NArg(), reportUsage)
		return 2
	}
	r, err := report.Build(report.Options{Dir: dir, Env: env, Root: *root, A: fs.Arg(0), B: fs.Arg(1)})
	if err != nil {
		fmt.Fprintf(stderr, "hypercase report: %v\n", err)
		return 1
	}
	if *text {
		err = r.WriteText(stdout)
	} else {
		err = r.WriteHTML(stdout)
	}
	if err != nil {
		fmt.Fprintf(stderr, "hypercase report: writing the report: %v\n", err)
		return 1
	}
	return 0
}
