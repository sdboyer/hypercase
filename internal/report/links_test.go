package report

import (
	"slices"
	"testing"

	"github.com/sdboyer/hypercase/internal/registry"
)

// TestLinks: each match links to the URL its template makes. Where
// matches overlap, the first to start wins, then the longer, then the
// pattern that sorts first. A match of no text links nothing, and a
// pattern given twice links once.
func TestLinks(t *testing.T) {
	l, bad := newLinker([]registry.IssueLink{
		{Pattern: `sheets#(\d+)`, URL: "https://example.com/sheets/$1"},
		{Pattern: `#(?P<n>\d+)`, URL: "https://example.com/any/${n}"},
		{Pattern: `sheets#\d+`, URL: "https://example.com/plain"},
		{Pattern: `sheets#(\d+)`, URL: "https://example.com/sheets/$1"},
		{Pattern: `x*`, URL: "https://example.com/empty"},
	})
	if bad != nil {
		t.Fatal(bad)
	}
	got := l.text("See sheets#3, and #4.")
	want := Text{
		{Text: "See "},
		{Text: "sheets#3", URL: "https://example.com/sheets/3"},
		{Text: ", and "},
		{Text: "#4", URL: "https://example.com/any/4"},
		{Text: "."},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := l.text("nothing here"); !slices.Equal(got, Text{{Text: "nothing here"}}) {
		t.Errorf("got %q for text with no match", got)
	}
	if got := l.text(""); got != nil {
		t.Errorf("got %q for no text", got)
	}
	// A pattern that isn't a regular expression is left out, and returned
	// once, however many times it is given; the others still link.
	l, bad = newLinker([]registry.IssueLink{{Pattern: `(`, URL: "u"}, {Pattern: `(`, URL: "v"}, {Pattern: `sheets#\d+`, URL: "w"}})
	if len(bad) != 1 || bad[0].pattern != `(` || bad[0].err == nil {
		t.Errorf("a pattern that doesn't compile: got %v", bad)
	}
	if got := l.text("sheets#1 ("); !slices.Equal(got, Text{{Text: "sheets#1", URL: "w"}, {Text: " ("}}) {
		t.Errorf("with a bad pattern left out, got %q", got)
	}
}
