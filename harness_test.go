package hypercase_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/diff"
)

// The self-tests run hypercase on fixtures, small archives under
// testdata/fixtures/<fixture>/, which fail on purpose, so only these tests
// open them. Each test copies its fixture to a temp dir and runs this
// package's own test binary there, as a child process. The child registers
// the fixture's declarations, chosen by name through fixtureEnv when the
// package is initialized, and its TestCorpus runs hypercase.Test. So a
// refusal at registration is observed as the child's panic, and a failure as
// the child's test output.
//
// Each test records what it ran and saw as a transcript, compared with
// testdata/transcripts/<test>.txt. HYPERCASE_UPDATE_GOLDEN=1 go test ./...
// rewrites the transcripts.

const (
	fixtureEnv     = "HYPERCASE_SELFTEST"
	reverseEnv     = "HYPERCASE_SELFTEST_REVERSE"
	observeEnv     = "HYPERCASE_SELFTEST_OBSERVE"
	fixtureUpdate  = "FIXTURE_UPDATE_GOLDEN"
	transcriptsVar = "HYPERCASE_UPDATE_GOLDEN"
)

func init() {
	name := os.Getenv(fixtureEnv)
	if name == "" {
		return
	}
	register, ok := registrations[name]
	if !ok {
		register, ok = liftRegistrations[name]
	}
	if !ok {
		register, ok = rawRegistrations[name]
	}
	if !ok {
		register, ok = transformRegistrations[name]
	}
	if !ok {
		register, ok = outcomeRegistrations[name]
	}
	if !ok {
		register, ok = chainRegistrations[name]
	}
	if !ok {
		register, ok = topologyRegistration(name)
	}
	if !ok {
		register, ok = cycleRegistrations[name]
	}
	if !ok {
		register, ok = stripRegistrations[name]
	}
	if !ok {
		register, ok = inputsRegistrations[name]
	}
	if !ok {
		panic("selftest: no registration named " + name)
	}
	register()
}

// TestCorpus is the child's test. In the parent it does nothing.
func TestCorpus(t *testing.T) {
	if os.Getenv(fixtureEnv) == "" {
		t.Skip("runs in a self-test's child process")
	}
	if during, ok := duringTest[os.Getenv(fixtureEnv)]; ok {
		during(t)
	}
	runTest(t)
	if len(runLog) > 0 {
		t.Logf("what ran, in order:\n%s", strings.Join(runLog, "\n"))
	}
}

// sandbox is one self-test's copy of its fixture.
type sandbox struct {
	t          *testing.T
	dir        string
	dirs       []string // the sandbox's paths, as given and with symlinks resolved, longest first
	transcript strings.Builder
}

func newSandbox(t *testing.T, fixture string) *sandbox {
	t.Helper()
	s := &sandbox{t: t, dir: t.TempDir()}
	// A message may name the sandbox by its path with symlinks resolved,
	// as on macOS, whose temp dirs are under /var, a link to /private/var.
	// The longer path is replaced first, so no part of it is left behind.
	real, err := filepath.EvalSymlinks(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	s.dirs = []string{s.dir}
	if real != s.dir {
		s.dirs = append(s.dirs, real)
	}
	slices.SortFunc(s.dirs, func(a, b string) int { return len(b) - len(a) })
	src := filepath.Join("testdata", "fixtures", fixture)
	if fixture != "" {
		err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(src, p)
			dst := filepath.Join(s.dir, rel)
			if d.IsDir() {
				return os.MkdirAll(dst, 0o755)
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(dst, data, 0o644)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(s.dir, "go.mod"), []byte("module example.com/fixture\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The root package's test file: the root-package check requires one
	// beside every registry. A fixture's other packages commit their own.
	if err := os.WriteFile(filepath.Join(s.dir, "corpus_test.go"), []byte(packageTestFile), 0o644); err != nil {
		t.Fatal(err)
	}
	return s
}

// packageTestFile marks a fixture's directory as a package with tests. The
// child is this package's test binary, so the file is never compiled.
const packageTestFile = "package fixture\n"

// result is what one child run left.
type result struct {
	out  string // the child's output, normalized
	code int    // its exit code
}

func (r result) ok() bool { return r.code == 0 }

// run runs the child in the package directory pkg, relative to the
// sandbox, with the named registration, as a normal run or an update.
func (s *sandbox) run(pkg, registration string, update bool, args ...string) result {
	s.t.Helper()
	var env []string
	if update {
		env = []string{fixtureUpdate + "=1"}
	}
	return s.runEnv(pkg, registration, env, args...)
}

// runEnv runs the child with extra environment variables.
func (s *sandbox) runEnv(pkg, registration string, extra []string, args ...string) result {
	s.t.Helper()
	r := s.exec(pkg, registration, extra, args...)
	s.record(pkg, registration, extra, args, r)
	return r
}

// parallel runs several children at once, as go test ./... runs packages,
// and records them in the order given.
func (s *sandbox) parallel(runs ...childRun) []result {
	s.t.Helper()
	results := s.execParallel(runs...)
	s.recordAll(runs, results)
	return results
}

// execParallel runs several children at once, and records nothing.
func (s *sandbox) execParallel(runs ...childRun) []result {
	results := make([]result, len(runs))
	done := make(chan struct{})
	for i, c := range runs {
		go func() {
			defer func() { done <- struct{}{} }()
			results[i] = s.exec(c.pkg, c.registration, c.env())
		}()
	}
	for range runs {
		<-done
	}
	return results
}

// recordAll records the children of a parallel run, in the order given.
func (s *sandbox) recordAll(runs []childRun, results []result) {
	for i, c := range runs {
		s.record(c.pkg, c.registration, c.env(), nil, results[i])
	}
}

// A childRun is one child of a parallel run.
type childRun struct {
	pkg, registration string
	update            bool
}

func (c childRun) env() []string {
	if c.update {
		return []string{fixtureUpdate + "=1"}
	}
	return nil
}

// exec runs the child, and returns its normalized output and exit code.
// It only reports a failure to start it, so it can run on any goroutine.
func (s *sandbox) exec(pkg, registration string, extra []string, args ...string) result {
	r := s.execRaw(pkg, registration, extra, args...)
	return result{out: s.shown(r), code: r.code}
}

// A rawResult is what one child run left: its output as the child wrote
// it, its exit code, and what kept it from starting, if anything did.
type rawResult struct {
	out  []byte
	code int
	err  error
}

func (r rawResult) ok() bool { return r.code == 0 }

// shown is a child's output, normalized, as exec gives it.
func (s *sandbox) shown(r rawResult) string {
	out := s.normalize(r.out)
	if r.err != nil {
		out += fmt.Sprintf("running the child: %v\n", r.err)
	}
	return out
}

// execRaw runs the child, as exec does, and returns what it left, its
// output as the child wrote it, for a caller that shows it only on a
// failure.
func (s *sandbox) execRaw(pkg, registration string, extra []string, args ...string) rawResult {
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestCorpus$", "-test.v", "-test.count=1"}, args...)...)
	cmd.Dir = filepath.Join(s.dir, pkg)
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if k != fixtureEnv && k != reverseEnv && k != observeEnv && k != fixtureUpdate && k != transcriptsVar && k != "GOFLAGS" && k != "PWD" {
			env = append(env, kv)
		}
	}
	env = append(env, fixtureEnv+"="+registration)
	env = append(env, extra...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	r := rawResult{out: out}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		r.code = exit.ExitCode()
	} else if err != nil {
		r.code, r.err = -1, err
	}
	return r
}

func (s *sandbox) record(pkg, registration string, extra, args []string, r result) {
	cmdline := strings.Join(append(slices.Clone(extra), "go test"), " ")
	if pkg != "" && pkg != "." {
		cmdline += " ./" + pkg
	}
	for _, a := range args {
		cmdline += " -" + strings.TrimPrefix(a, "-test.")
	}
	fmt.Fprintf(&s.transcript, "$ %s  # %s\n%s", cmdline, registration, r.out)
	if r.code != 0 {
		fmt.Fprintf(&s.transcript, "[exit %d]\n", r.code)
	}
}

func (s *sandbox) normal(registration string, args ...string) result {
	s.t.Helper()
	return s.run("", registration, false, args...)
}

func (s *sandbox) update(registration string, args ...string) result {
	s.t.Helper()
	return s.run("", registration, true, args...)
}

var (
	runLine    = regexp.MustCompile(`^=== (RUN|PAUSE|CONT|NAME)`)
	timing     = regexp.MustCompile(` \(\d+\.\d+s\)`)
	location   = regexp.MustCompile(`^(\s*)[\w.-]+\.go:\d+: `)
	stackStart = "stack of the panic:"
)

func (s *sandbox) normalize(out []byte) string {
	text := string(out)
	for _, dir := range s.dirs {
		text = strings.ReplaceAll(text, dir, "$SANDBOX")
	}
	if i := strings.Index(text, "\n\ngoroutine "); i >= 0 && strings.Contains(text[:i], "panic: ") {
		// A panic the framework didn't recover: its stack holds addresses.
		text = text[:i+1] + "[stack]\n"
	}
	var b strings.Builder
	skipIndent := -1
	for _, line := range strings.SplitAfter(text, "\n") {
		if line == "" {
			continue
		}
		if runLine.MatchString(line) || strings.HasPrefix(line, "coverage:") {
			continue
		}
		line = timing.ReplaceAllString(line, "")
		line = location.ReplaceAllString(line, "$1")
		ind := len(line) - len(strings.TrimLeft(line, " "))
		if skipIndent >= 0 {
			if ind > skipIndent {
				continue
			}
			skipIndent = -1
		}
		if strings.TrimSpace(line) == stackStart {
			b.WriteString(strings.Repeat(" ", ind) + stackStart + " [stack]\n")
			skipIndent = ind
			continue
		}
		b.WriteString(line)
	}
	return b.String()
}

func (s *sandbox) path(rel string) string { return filepath.Join(s.dir, filepath.FromSlash(rel)) }

// read returns a file in the sandbox.
func (s *sandbox) read(rel string) []byte {
	s.t.Helper()
	data, err := os.ReadFile(s.path(rel))
	if err != nil {
		s.t.Fatal(err)
	}
	return data
}

// write writes a file in the sandbox, as a person's edit, and notes it.
func (s *sandbox) write(rel string, data []byte, why string) {
	s.t.Helper()
	if err := os.MkdirAll(filepath.Dir(s.path(rel)), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(s.path(rel), data, 0o644); err != nil {
		s.t.Fatal(err)
	}
	fmt.Fprintf(&s.transcript, "$ edit %s  # %s\n", rel, why)
}

// cat records a file in the transcript.
func (s *sandbox) cat(rel string) {
	s.t.Helper()
	fmt.Fprintf(&s.transcript, "$ cat %s\n%s", rel, s.read(rel))
}

// remove removes a file in the sandbox, as a person's edit, and notes it.
func (s *sandbox) remove(rel, why string) {
	s.t.Helper()
	if err := os.Remove(s.path(rel)); err != nil {
		s.t.Fatal(err)
	}
	fmt.Fprintf(&s.transcript, "$ rm %s  # %s\n", rel, why)
}

// exists reports whether a file is in the sandbox.
func (s *sandbox) exists(rel string) bool {
	_, err := os.Stat(s.path(rel))
	return err == nil
}

// snapshot returns every file in the sandbox, by slash path.
func (s *sandbox) snapshot() map[string][]byte {
	s.t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(s.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(s.dir, p)
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		s.t.Fatal(err)
	}
	return files
}

// unchanged checks the sandbox holds the files of a snapshot, and no
// others.
func (s *sandbox) unchanged(before map[string][]byte) {
	s.t.Helper()
	after := s.snapshot()
	for _, p := range slices.Sorted(maps.Keys(before)) {
		data := before[p]
		if got, ok := after[p]; !ok {
			s.t.Errorf("%s was removed", p)
		} else if !bytes.Equal(got, data) {
			s.t.Errorf("%s changed (-before +after):\n%s", p, diff.Lines(data, got))
		}
	}
	for _, p := range slices.Sorted(maps.Keys(after)) {
		if _, ok := before[p]; !ok {
			s.t.Errorf("%s was created", p)
		}
	}
}

// note records a line in the transcript.
func (s *sandbox) note(format string, args ...any) {
	fmt.Fprintf(&s.transcript, "# "+format+"\n", args...)
}

// passes and fails check a run's result.
func (s *sandbox) passes(r result) {
	s.t.Helper()
	if !r.ok() {
		s.t.Errorf("the run failed, and should pass:\n%s", r.out)
	}
}

func (s *sandbox) fails(r result, want ...string) {
	s.t.Helper()
	if r.ok() {
		s.t.Errorf("the run passed, and should fail:\n%s", r.out)
	}
	s.contains(r, want...)
}

// contains checks a run's output holds each string. Lines are compared
// with their leading spaces removed, since go test indents a message's
// lines after the first.
func (s *sandbox) contains(r result, want ...string) {
	s.t.Helper()
	for _, w := range want {
		if !strings.Contains(flat(r.out), flat(w)) {
			s.t.Errorf("the output doesn't hold %q:\n%s", w, r.out)
		}
	}
}

func (s *sandbox) lacks(r result, unwanted ...string) {
	s.t.Helper()
	for _, w := range unwanted {
		if strings.Contains(flat(r.out), flat(w)) {
			s.t.Errorf("the output holds %q, and shouldn't:\n%s", w, r.out)
		}
	}
}

func flat(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimLeft(l, " ")
	}
	return strings.Join(lines, "\n")
}

// same checks a file's bytes are what they were.
func (s *sandbox) same(rel string, before []byte) {
	s.t.Helper()
	if got := s.read(rel); !bytes.Equal(got, before) {
		s.t.Errorf("%s changed (-before +after):\n%s", rel, diff.Lines(before, got))
	}
}

// done compares the transcript with its golden.
func (s *sandbox) done() {
	s.t.Helper()
	name := strings.ReplaceAll(s.t.Name(), "/", "-") + ".txt"
	golden := filepath.Join("testdata", "transcripts", name)
	got := []byte(s.transcript.String())
	if os.Getenv(transcriptsVar) == "1" {
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			s.t.Fatal(err)
		}
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			s.t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		s.t.Fatalf("%v\nRun %s=1 go test ./... to write the transcript, and read it.", err, transcriptsVar)
	}
	if !bytes.Equal(got, want) {
		s.t.Errorf("%s differs (-golden +got):\n%s\nIf the change is intended, run %s=1 go test ./... and read the diff.", golden, diff.Lines(want, got), transcriptsVar)
	}
}
