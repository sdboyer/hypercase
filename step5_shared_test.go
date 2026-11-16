package hypercase_test

import (
	"bytes"
	"strings"
	"testing"
)

// Step 5: shared corpora. In the fixture shared, the root package, ., runs
// echo, and ./b runs shout, over one corpus, testdata/script/.

const twice = "testdata/script/twice.txtar"

// stripped is twice with neither package's sections.
func stripped(s *sandbox) []byte {
	s.t.Helper()
	data := s.read(twice)
	return data[:bytes.Index(data, []byte("-- out/"))]
}

func TestSharedCorpus(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	// A normal run only reads: it takes no lock and writes nothing.
	before := s.snapshot()
	s.passes(s.normal("pkg-a"))
	s.passes(s.run("b", "pkg-b", false))
	s.unchanged(before)

	// Each package judges only its own sections.
	edited := strings.Replace(string(s.read(twice)), "-- out/shout/values.txt --\nA1 SIX\n", "-- out/shout/values.txt --\nA1 SIX?\n", 1)
	edited = strings.Replace(edited, "-- in/do/out/values.txt --\nA1 6\n", "-- in/do/out/values.txt --\nA1 7\n", 1)
	s.write(twice, []byte(edited), "edit b's section by hand, and change what echo prints")
	s.fails(s.run("b", "pkg-b", false), "out/shout/values.txt differs")
	s.passes(s.update("pkg-a"))
	got := s.read(twice)
	if !bytes.Contains(got, []byte("-- out/echo/values.txt --\nA1 7\n")) || !bytes.Contains(got, []byte("-- out/shout/values.txt --\nA1 SIX?\n")) {
		t.Errorf("a's update should write its own section and leave b's as edited:\n%s", got)
	}
	s.write(twice, []byte(strings.Replace(string(got), "A1 SIX?\n", "A1 SIX\n", 1)), "restore b's section")

	// Two packages' updates give the same archive in either order, and
	// run at once, as go test ./... runs them.
	bare := stripped(s)
	s.write(twice, bare, "strip both packages' sections")
	s.passes(s.update("pkg-a"))
	s.passes(s.run("b", "pkg-b", true))
	ab := s.read(twice)
	s.write(twice, bare, "strip both packages' sections")
	s.passes(s.run("b", "pkg-b", true))
	s.passes(s.update("pkg-a"))
	s.same(twice, ab)
	s.write(twice, bare, "strip both packages' sections")
	for _, r := range s.parallel(childRun{".", "pkg-a", true}, childRun{"b", "pkg-b", true}) {
		s.passes(r)
	}
	s.same(twice, ab)
	s.cat(twice)
	if !s.exists("testdata/.hypercase.lock") {
		t.Error("the updates left no lock file at testdata/.hypercase.lock")
	}

	// One package's update puts another's section in its place, and never
	// changes its bytes.
	moved := strings.Replace(string(ab), "-- out/echo/values.txt --\nA1 7\n", "", 1) + "-- out/echo/values.txt --\nA1 7\n"
	moved = strings.Replace(moved, "-- out/shout/values.txt --\nA1 SIX\n", "", 1)
	moved = strings.Replace(moved, "-- out/echo/", "-- out/shout/values.txt --\nA1 SIX\n-- out/echo/", 1)
	s.write(twice, []byte(moved), "move b's section before a's")
	s.passes(s.run("b", "pkg-b", false))
	s.fails(s.normal("pkg-a"), "testdata/script/twice.txtar has update-written sections out of their place.\nRun FIXTURE_UPDATE_GOLDEN=1 go test ./... to put them in place.")
	s.passes(s.update("pkg-a"))
	s.same(twice, ab)
	s.passes(s.normal("pkg-a"))
	s.done()
}

// TestUpdateRereads: an update re-reads each archive under the lock, and
// writes nothing to one whose hand-written sections changed since its
// cells ran on them.
func TestUpdateRereads(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "shared")
	bare := string(stripped(s))
	// A gaps line names a column, so the edit keeps the section well formed.
	for _, edit := range []string{"in/note/text", "comment", "properties", "gaps\nformat A1 prints as =6. Waits on sheets#8."} {
		section, _, _ := strings.Cut(edit, "\n")
		archive := bare + "-- in/do/edit --\n" + edit + "\n"
		archive = strings.Replace(archive, "-- in/do/out/", "-- in/note/text --\nA note.\n-- properties --\ncircular\n-- gaps --\nshout A1 is SIX. Waits on sheets#14.\n-- in/do/out/", 1)
		s.write(twice, []byte(archive), "echo's run adds a line to "+section+", as a person's edit during the update would")
		s.fails(s.update("pkg-a"), "testdata/script/twice.txtar changed while the update ran: its comment or hand-written sections differ from those its cells ran on, so the update wrote nothing to it.\nRun FIXTURE_UPDATE_GOLDEN=1 go test ./... again.")
		if bytes.Contains(s.read(twice), []byte("-- out/echo/")) {
			t.Errorf("the update wrote to an archive whose %s changed", section)
		}
		// The edit is made once, so the next update completes.
		s.passes(s.update("pkg-a"))
	}
	s.cat(twice)

	// A change to another package's sections doesn't stop the update.
	s.write(twice, []byte(bare+"-- in/do/edit --\nout/shout/values.txt\n-- out/shout/values.txt --\nA1 SIX\n"), "echo's run adds a line to b's section")
	s.passes(s.update("pkg-a"))
	s.cat(twice)
	got := s.read(twice)
	if !bytes.Contains(got, []byte("-- out/echo/values.txt --\nA1 6\n")) || !bytes.Contains(got, []byte("-- out/shout/values.txt --\nA1 SIX\nedited\n")) {
		t.Errorf("the update should write a's section, and keep b's as edited:\n%s", got)
	}
	s.done()
}
