package hypercase_test

import "testing"

// TestIssueLinks: the registry records the project's issue links, sorted,
// so hypercase report can read them, and a change to them changes the
// registry.
func TestIssueLinks(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "basic")
	s.passes(s.update("echo-links"))
	s.cat("testdata/hypercase.json")
	s.passes(s.normal("echo-links"))
	s.fails(s.normal("echo"), "testdata/hypercase.json differs from what the package registers (-committed +registered):", `-       "pattern": "recipes#(\\d+)",`)
	s.done()
}

// TestIssueLinkRefusals: a link whose pattern doesn't compile, or that has
// no URL, is refused at registration, whichever cells -run selects.
func TestIssueLinkRefusals(t *testing.T) {
	t.Parallel()
	s := newSandbox(t, "basic")
	for _, tt := range []struct{ reg, want string }{
		{"bad-link-pattern", `panic: hypercase: the project's issue link pattern "sheets#(\\d+" is not a regular expression:`},
		{"link-no-url", `panic: hypercase: the project's issue link pattern "sheets#(\\d+)" has no URL.`},
	} {
		r := s.normal(tt.reg)
		s.fails(r, tt.want)
		s.lacks(r, "--- ")
		s.fails(s.normal(tt.reg, "-test.run=^$"), tt.want)
	}
	s.done()
}
