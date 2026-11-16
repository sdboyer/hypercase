package hypercase_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
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
}

// sandbox is one self-test's copy of its fixture.
type sandbox struct {
	t          *testing.T
	dir        string
	transcript strings.Builder
}

func newSandbox(t *testing.T, fixture string) *sandbox {
	t.Helper()
	s := &sandbox{t: t, dir: t.TempDir()}
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
	return s
}

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
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestCorpus$", "-test.v", "-test.count=1"}, args...)...)
	cmd.Dir = filepath.Join(s.dir, pkg)
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if k != fixtureEnv && k != fixtureUpdate && k != transcriptsVar && k != "GOFLAGS" {
			env = append(env, kv)
		}
	}
	env = append(env, fixtureEnv+"="+registration)
	env = append(env, extra...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	r := result{out: s.normalize(out)}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		r.code = exit.ExitCode()
	} else if err != nil {
		s.t.Fatalf("running the child: %v", err)
	}

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
	return r
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
	text := strings.ReplaceAll(string(out), s.dir, "$SANDBOX")
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
