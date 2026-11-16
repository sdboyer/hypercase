// Command hypercase is hypercase's command line.
//
// Usage:
//
//	hypercase report [-root dir] [-text] <a> <b>
//	hypercase strip [-root dir] [-column name] [-transform name] [-frozen]
//
// report compares two git revisions of a corpus root, read from the
// repository's object store, never the working tree. It writes a static
// HTML page on stdout: the cells that changed, ranked, with changed
// outcomes first, then cells that appeared or disappeared, then files that
// changed under the same outcome; and the matrix of each corpus at <b>.
// With -text it writes the ranked changes as plain text instead.
//
// strip removes update-written sections from the archives of a corpus
// root, telling them from hand-written ones by name alone, and leaves every
// hand-written section's bytes as they were. A person runs it, never an
// update; the update that follows writes the sections again from the
// inputs and the code. The archives are the *.txtar files directly in the
// directories directly under the corpus root's testdata/. With no flags it
// strips every archive, and removes the registries that record the corpus
// root and the findings files in their packages' testdata/checks/. It
// keeps a package's registry while a findings file of the package is
// left, so that strip run again finds it.
// -column limits it to a column's sections, at every level, and -transform
// to those under a transform's variant, transform/<name>/; given together,
// they limit it to the column's sections under that variant. Either leaves
// the registries and findings files as they are. It leaves a frozen cell's
// sections in place, and lists the cell; -frozen strips them too. It
// prints a line for each archive it changed, each frozen cell it left and
// each file it left with a problem, then the update to run. A line that
// says to run strip again gives the command, with -root unless strip ran
// at the module root on the module-root corpus.
//
// strip exits 1 having removed nothing when it can't find the corpus root
// or the module root, or the corpus root holds no testdata/; when a
// registry under the module can't be read, or the registries that record
// the corpus root record different update variables; when reading the
// corpus root's testdata/ fails; and when -column or -transform matches no
// section that strip removes. It exits 1 too when it left an archive or a
// file in place for a problem, a failed write or removal included. It
// exits 2 for a usage error: a flag given twice, an unknown flag, an
// argument, an empty -root, or an empty or ill-formed -column or
// -transform.
//
// -root names the corpus root, relative to the current directory. Without
// it, the corpus root is the one the registry of the package in the
// current directory records, or, at the module root with no registry
// there, the module root. Anywhere else with no registry, strip asks for
// -root.
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
