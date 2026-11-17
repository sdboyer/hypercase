package report

import (
	"testing"

	"github.com/sdboyer/hypercase/internal/diff"
)

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
