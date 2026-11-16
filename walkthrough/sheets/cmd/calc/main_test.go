package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name   string
		cells  map[string]string
		code   int
		stdout string
		stderr string
	}{
		{"empty", map[string]string{}, 0, "", ""},
		{"twice", map[string]string{"A1": "=B1*2\n", "B1": "3\n"}, 0, "6,3\n", ""},
		{"divides by zero", map[string]string{"A1": "1\n", "B1": "0\n", "C1": "=A1/B1\n"}, 0,
			"1,0,#DIV/0!\n", "C1: divides by zero\n"},
		{"malformed", map[string]string{"A1": "4\n", "D1": "total\n"}, 1, "",
			"calc: D1: not a number or a formula: \"total\"\n"},
		{"cycle", map[string]string{"A1": "=B1\n", "B1": "=A1\n"}, 1, "", "calc: A1 -> B1 -> A1: cycle\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, data := range tt.cells {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var stdout, stderr strings.Builder
			if code := run([]string{dir}, &stdout, &stderr); code != tt.code {
				t.Errorf("exit status: got %d, want %d", code, tt.code)
			}
			if got := stdout.String(); got != tt.stdout {
				t.Errorf("stdout: got %q, want %q", got, tt.stdout)
			}
			if got := stderr.String(); got != tt.stderr {
				t.Errorf("stderr: got %q, want %q", got, tt.stderr)
			}
		})
	}
}

func TestRunArgs(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "A1")
	if err := os.WriteFile(file, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing")
	tests := []struct {
		name   string
		args   []string
		code   int
		stderr string // a prefix of stderr
	}{
		{"no sheet", nil, 2, "usage: calc DIR\n"},
		{"two sheets", []string{dir, dir}, 2, "usage: calc DIR\n"},
		{"missing", []string{missing}, 1, "calc: stat " + missing + ": "},
		{"not a directory", []string{file}, 1, "calc: " + file + " is not a directory\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			if code := run(tt.args, &stdout, &stderr); code != tt.code {
				t.Errorf("exit status: got %d, want %d", code, tt.code)
			}
			if !strings.HasPrefix(stderr.String(), tt.stderr) {
				t.Errorf("stderr: got %q, want it to start %q", stderr.String(), tt.stderr)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout: got %q, want nothing", stdout.String())
			}
		})
	}
}

// The mount serves the directory at sheet/ and nothing else.
func TestMount(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "A1"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := mount{os.DirFS(dir)}
	for _, name := range []string{"sheet", "sheet/A1"} {
		f, err := m.Open(name)
		if err != nil {
			t.Errorf("Open(%q): %v", name, err)
			continue
		}
		f.Close()
	}
	for _, name := range []string{".", "A1", "sheets/A1", "sheet/../A1", "sheet/"} {
		if f, err := m.Open(name); err == nil {
			f.Close()
			t.Errorf("Open(%q) succeeded", name)
		}
	}
}
