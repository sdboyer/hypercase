package sheetcorpus_test

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/sdboyer/hypercase/archive"
	"github.com/sdboyer/hypercase/walkthrough/sheets/step10/sheetcorpus"
)

// TestIssueLinks: the project's issue links link each of negative's gaps
// lines, which wait on sheets#3, to that issue, as the report links a gaps
// line: each match of a pattern, expanded by its URL template.
func TestIssueLinks(t *testing.T) {
	data, err := os.ReadFile("../testdata/sheet/negative.txtar")
	if err != nil {
		t.Fatal(err)
	}
	gaps := archive.Parse(data).Gaps()
	if len(gaps) == 0 {
		t.Fatal("negative has no gaps lines")
	}
	for _, g := range gaps {
		var urls []string
		for _, l := range sheetcorpus.Project.IssueLinks {
			re := regexp.MustCompile(l.Pattern)
			for _, m := range re.FindAllStringSubmatchIndex(g.Text, -1) {
				urls = append(urls, string(re.ExpandString(nil, l.URL, g.Text, m)))
			}
		}
		if want := []string{"https://example.com/sheets/issues/3"}; !slices.Equal(urls, want) {
			t.Errorf("%s: links %q, want %q", g.Cell(), urls, want)
		}
	}
}
