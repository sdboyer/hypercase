package report

import (
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/diff"
)

// TestHTMLChangeGap: a changed cell that a gaps line marks shows the line
// under the change, marked as a known gap, with its issue linked.
func TestHTMLChangeGap(t *testing.T) {
	r := &Report{Changes: []Change{{
		Rank:   OutcomeChanged,
		Case:   "sheet/negative",
		Column: "parse",
		Before: "rejected S101",
		After:  "accepted",
		Gap: Text{
			{Text: "A negative number is valid. Waits on "},
			{Text: "sheets#3", URL: "https://example.com/sheets/issues/3"},
			{Text: "."},
		},
	}}}
	var b strings.Builder
	if err := r.WriteHTML(&b); err != nil {
		t.Fatal(err)
	}
	const want = `<p class="gap"><span class="badge">known gap</span> A negative number is valid. Waits on <a href="https://example.com/sheets/issues/3">sheets#3</a>.</p>
</li>`
	if !strings.Contains(b.String(), want) {
		t.Errorf("the page doesn't show the change's gap as\n%s\nIt is:\n%s", want, b.String())
	}
}

// TestDiffLine: the line for a stretch the diff can't align says how many
// lines it leaves out of each side, and, as a removed or added line does,
// which of its sides leaves out a last line with no final newline.
func TestDiffLine(t *testing.T) {
	const note = "  ... left out: 1 line before and 2 lines after, which the diff can't align"
	for _, tt := range []struct {
		line        diff.Line
		class, text string
	}{
		{diff.Line{Kind: diff.Unaligned, Old: 1, New: 2}, "unaligned", note},
		{diff.Line{Kind: diff.Unaligned, Old: 1, New: 2, OldNoNewline: true}, "unaligned", note + "\n- \\ no final newline"},
		{diff.Line{Kind: diff.Unaligned, Old: 1, New: 2, NewNoNewline: true}, "unaligned", note + "\n+ \\ no final newline"},
		{diff.Line{Kind: diff.Unaligned, Old: 1, New: 2, OldNoNewline: true, NewNoNewline: true}, "unaligned", note + "\n- \\ no final newline\n+ \\ no final newline"},
		{diff.Line{Kind: '-', Text: "a", NoNewline: true}, "del", "- a\n- \\ no final newline"},
	} {
		if class, text := diffLine(tt.line); class != tt.class || text != tt.text {
			t.Errorf("%+v: got %s %q, want %s %q", tt.line, class, text, tt.class, tt.text)
		}
	}
}
