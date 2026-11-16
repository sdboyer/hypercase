package main

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/sdboyer/hypercase/internal/gittest"
)

// section returns the part of a page from the first occurrence of start
// to the next occurrence of end after it.
func section(t *testing.T, page, start, end string) string {
	t.Helper()
	i := strings.Index(page, start)
	if i < 0 {
		t.Fatalf("the page doesn't hold %q", start)
	}
	j := strings.Index(page[i:], end)
	if j < 0 {
		t.Fatalf("the page doesn't hold %q after %q", end, start)
	}
	return page[i : i+j]
}

// matrixRow returns the head of a case's row in a corpus's matrix, and
// its cells, one per column.
func matrixRow(t *testing.T, page, corpus, name string) (head string, cells []string) {
	t.Helper()
	m := section(t, page, `<section class="matrix" id="corpus-`+corpus+`">`, "</section>")
	row := section(t, m, `<th scope="row"><span class="case">`+name+`</span>`, "</tr>")
	parts := strings.Split(row, "<td ")
	return parts[0], parts[1:]
}

// change returns a cell's entry in the ranked changes.
func change(t *testing.T, page, name, column string) string {
	t.Helper()
	changes := section(t, page, `<section id="changes">`, "</section>")
	entry := section(t, changes, `<span class="case">`+name+`</span> <span class="col">`+column+`</span>`, "</li>\n<li class=\"change")
	if i := strings.Index(entry, "</ul>\n<h3"); i >= 0 {
		entry = entry[:i]
	}
	return entry
}

func holds(t *testing.T, what, s string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(s, w) {
			t.Errorf("%s doesn't hold %q:\n%s", what, w, s)
		}
	}
}

func lacks(t *testing.T, what, s string, unwanted ...string) {
	t.Helper()
	for _, w := range unwanted {
		if strings.Contains(s, w) {
			t.Errorf("%s holds %q, and shouldn't:\n%s", what, w, s)
		}
	}
}

// TestPage: without -text, the report is one HTML page on stdout, which
// loads nothing.
func TestPage(t *testing.T) {
	t.Parallel()
	g, a, b := sheets(t)
	page := ok(t, g, "step09", "report", a, b)
	if !strings.HasPrefix(page, "<!doctype html>\n") || !strings.HasSuffix(page, "</html>\n") {
		t.Errorf("the page is not one HTML document:\n%s", page)
	}
	lacks(t, "the page", page, "<script", "<link", "<img", "<iframe", "<object", "<embed", " src=", "@import", "url(")
	for _, m := range regexp.MustCompile(`href="([^"]*)"`).FindAllStringSubmatch(page, -1) {
		if !strings.HasPrefix(m[1], "https://example.com/sheets/issues/") {
			t.Errorf("the page links to %s, which is no issue", m[1])
		}
	}
	holds(t, "the page", page, "<code>"+a[:12]+"</code>", "<code>"+b[:12]+"</code>", "<code>step09</code>")
	// What the page shows is escaped.
	head, _ := matrixRow(t, page, "sheet", "sheet/ratio")
	holds(t, "ratio's row", head, "a ratio &lt;1.")
}

// TestPageMatrix: the page shows the matrix of each corpus at b: its
// columns, its cases, and each cell's outcome, with the cells that changed
// marked by how. A case only at a is not in it.
func TestPageMatrix(t *testing.T) {
	t.Parallel()
	g, a, b := sheets(t)
	page := ok(t, g, "step09", "report", a, b)
	m := section(t, page, `<section class="matrix" id="corpus-sheet">`, "</section>")
	var cols []string
	for _, c := range regexp.MustCompile(`<th scope="col"[^>]*>([^<]*)</th>`).FindAllStringSubmatch(m, -1) {
		cols = append(cols, c[1])
	}
	if want := []string{"case", "calc", "evaluate", "format", "parse", "resolve"}; !slices.Equal(cols, want) {
		t.Errorf("the matrix's columns are %q, want %q", cols, want)
	}
	var rows []string
	for _, r := range regexp.MustCompile(`<th scope="row"><span class="case">([^<]*)</span>`).FindAllStringSubmatch(m, -1) {
		rows = append(rows, r[1])
	}
	want := []string{"sheet/div", "sheet/loop", "sheet/negative", "sheet/ratio", "sheet/twice", "sheet/twice/variant/cycle", "sheet/twice/transform/looped"}
	if !slices.Equal(rows, want) {
		t.Errorf("the matrix's rows are %q, want %q", rows, want)
	}

	_, cells := matrixRow(t, page, "sheet", "sheet/loop")
	holds(t, "loop's calc", cells[0], `class="cell s-rejected bytes-changed"`, "rejected S201")
	holds(t, "loop's evaluate", cells[1], `class="cell s-blocked"`, "blocked resolve")
	_, cells = matrixRow(t, page, "sheet", "sheet/div")
	holds(t, "div's evaluate", cells[1], `class="cell s-accepted outcome-changed"`, "accepted div-zero")
	holds(t, "div's format", cells[2], `class="cell s-empty"`)
	_, cells = matrixRow(t, page, "sheet", "sheet/ratio")
	holds(t, "ratio's parse", cells[3], `class="cell s-accepted appeared"`)
	_, cells = matrixRow(t, page, "sheet", "sheet/twice")
	lacks(t, "twice's calc", cells[0], "changed", "appeared")
	lacks(t, "the matrix", m, "sheet/blank")

	dl := section(t, m, `<dl class="columns">`, "</dl>")
	holds(t, "the columns' descriptions", dl, "<dt>parse</dt><dd>Parses every cell.</dd>")
}

// TestPageProperties: each case's stated properties are shown beside it,
// in the matrix and in the changes, each with its description. A variant
// states its own.
func TestPageProperties(t *testing.T) {
	t.Parallel()
	g, a, b := sheets(t)
	page := ok(t, g, "step09", "report", a, b)
	const circular = `<span class="prop" title="Some cell&#39;s formula refers back to itself, directly or through other cells.">circular</span>`
	head, _ := matrixRow(t, page, "sheet", "sheet/loop")
	holds(t, "loop's row", head, circular)
	head, _ = matrixRow(t, page, "sheet", "sheet/twice/variant/cycle")
	holds(t, "twice/variant/cycle's row", head, circular)
	head, _ = matrixRow(t, page, "sheet", "sheet/twice")
	lacks(t, "twice's row", head, `class="prop"`)
	holds(t, "loop calc's change", change(t, page, "sheet/loop", "calc"), circular)
	lacks(t, "div calc's change", change(t, page, "sheet/div", "calc"), `class="prop"`)
}

// TestPageGaps: each gaps line is shown on the cell it marks, linked to
// its issue, and marks a cell with committed files frozen. A transform's
// line, in its base's gaps, marks the transform's variant's cell.
func TestPageGaps(t *testing.T) {
	t.Parallel()
	g, a, b := sheets(t)
	page := ok(t, g, "step09", "report", a, b)
	_, cells := matrixRow(t, page, "sheet", "sheet/negative")
	holds(t, "negative's evaluate", cells[1], "frozen", `<div class="gap">A negative number is valid, and evaluates to itself. Waits on <a href="https://example.com/sheets/issues/3">sheets#3</a>.</div>`)
	holds(t, "negative's calc", cells[0], `class="cell s-empty"`, `<div class="gap">A negative number is valid. Waits on <a href="https://example.com/sheets/issues/3">sheets#3</a>.</div>`)
	lacks(t, "negative's calc", cells[0], "frozen")
	lacks(t, "negative's format", cells[2], "gap")

	_, cells = matrixRow(t, page, "sheet", "sheet/twice/transform/looped")
	holds(t, "twice/transform/looped's format", cells[2], "frozen", `Waits on <a href="https://example.com/sheets/issues/7">sheets#7</a>.`)
	_, cells = matrixRow(t, page, "sheet", "sheet/twice")
	lacks(t, "twice's format", cells[2], "frozen", "gap")
}

// TestPageGapWithoutCell: a gaps line is shown on the cell it marks even
// where the cell, or its case, has no sections.
func TestPageGapWithoutCell(t *testing.T) {
	t.Parallel()
	g := gittest.New(t)
	c := revisions(t, g, module(nil), module(map[string]string{
		"x": "A case.\n-- in/sheet/A1 --\n1\n-- gaps --\ntransform/doubled evaluate Waits on sheets#5.\n-- out/calc/outcome --\naccepted\n",
	}))
	page := ok(t, g, "", "report", c[0], c[1])
	_, cells := matrixRow(t, page, "sheet", "sheet/x/transform/doubled")
	holds(t, "x/transform/doubled's evaluate", cells[1], `class="cell s-empty"`, `Waits on <a href="https://example.com/sheets/issues/5">sheets#5</a>.`)
	lacks(t, "x/transform/doubled's evaluate", cells[1], "frozen")
}

// TestPageLinks: text that matches a link pattern, in a gaps line or a
// description, links to the URL the pattern's template makes, through the
// patterns the registries record at b.
func TestPageLinks(t *testing.T) {
	t.Parallel()
	g, a, b := sheets(t)
	page := ok(t, g, "step09", "report", a, b)
	head, _ := matrixRow(t, page, "sheet", "sheet/negative")
	holds(t, "negative's description", head, `which <a href="https://example.com/sheets/issues/3">sheets#3</a> is about.`)
	holds(t, "calc's description", page, `<dd>Runs the calc command on the sheet; see <a href="https://example.com/sheets/issues/12">sheets#12</a>.</dd>`,
		`title="Runs the calc command on the sheet; see sheets#12."`)
	// a's registries link to another URL.
	lacks(t, "the page", page, "old.example.com")
	// The report from b to a links through a's.
	back := ok(t, g, "step09", "report", b, a)
	holds(t, "the page from b to a", back, `<a href="https://old.example.com/3">sheets#3</a>`)
}
