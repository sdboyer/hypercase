// Command hypercase is hypercase's command line.
//
// Usage:
//
//	hypercase report [-root dir] [-text] <a> <b>
//
// report compares two git revisions of a corpus root, read from the
// repository's object store, never the working tree. It writes a static
// HTML page on stdout: the cells that changed, ranked, with changed
// outcomes first, then cells that appeared or disappeared, then files that
// changed under the same outcome; and the matrix of each corpus at <b>.
// With -text it writes the ranked changes as plain text instead.
//
// -root names the corpus root, relative to the current directory. Without
// it, the corpus root is found as a test finds it: the one the registry of
// the package in the current directory records, or else the module root.
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

const usage = `usage: hypercase report [-root dir] [-text] <a> <b>`

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
	}
	fmt.Fprintf(stderr, "hypercase: no command %q\n%s\n", args[0], usage)
	return 2
}

func runReport(dir string, env []string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, usage)
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
		fmt.Fprintf(stderr, "hypercase report takes two revisions, and was given %d.\n%s\n", fs.NArg(), usage)
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
