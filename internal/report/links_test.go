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
	l, err := newLinker([]registry.IssueLink{
		{Pattern: `sheets#(\d+)`, URL: "https://example.com/sheets/$1"},
		{Pattern: `#(?P<n>\d+)`, URL: "https://example.com/any/${n}"},
		{Pattern: `sheets#\d+`, URL: "https://example.com/plain"},
		{Pattern: `sheets#(\d+)`, URL: "https://example.com/sheets/$1"},
		{Pattern: `x*`, URL: "https://example.com/empty"},
	})
	if err != nil {
		t.Fatal(err)
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
	if _, err := newLinker([]registry.IssueLink{{Pattern: `(`, URL: "u"}}); err == nil {
		t.Error("a pattern that doesn't compile: got no error")
	}
}
