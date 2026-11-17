package report

import (
	"cmp"
	"regexp"
	"slices"

	"github.com/sdboyer/hypercase/internal/registry"
)

// Text is text that may link to issues: a run of segments, each plain or
// linked.
type Text []Segment

// A Segment is a piece of text, and the URL it links to, or "".
type Segment struct {
	Text, URL string
}

// linker links the text that names an issue to the issue, through a
// project's link patterns.
type linker struct {
	links []link
}

type link struct {
	re  *regexp.Regexp
	url string
}

// newLinker compiles link patterns, each once, sorted by pattern and then
// URL, which is the order that breaks a tie between two matches of one
// length at one place. It leaves out each pattern that isn't a regular
// expression, and returns it, with its error.
func newLinker(links []registry.IssueLink) (*linker, []badLink) {
	links = slices.Clone(links)
	slices.SortFunc(links, func(a, b registry.IssueLink) int {
		return cmp.Or(cmp.Compare(a.Pattern, b.Pattern), cmp.Compare(a.URL, b.URL))
	})
	links = slices.Compact(links)
	l := &linker{}
	var bad []badLink
	for _, il := range links {
		re, err := regexp.Compile(il.Pattern)
		if err != nil {
			if len(bad) == 0 || bad[len(bad)-1].pattern != il.Pattern {
				bad = append(bad, badLink{pattern: il.Pattern, err: err})
			}
			continue
		}
		l.links = append(l.links, link{re: re, url: il.URL})
	}
	return l, bad
}

// A badLink is a link pattern that isn't a regular expression.
type badLink struct {
	pattern string
	err     error
}

// text links each match in s to the URL its pattern's template makes from
// it. Where matches overlap, the one that starts first wins, then the
// longer, then the one whose pattern sorts first. A match of no text links
// nothing.
func (l *linker) text(s string) Text {
	if s == "" {
		return nil
	}
	type match struct {
		start, end int
		link       int
		idx        []int
	}
	var ms []match
	for i, lk := range l.links {
		for _, idx := range lk.re.FindAllStringSubmatchIndex(s, -1) {
			if idx[0] < idx[1] {
				ms = append(ms, match{start: idx[0], end: idx[1], link: i, idx: idx})
			}
		}
	}
	slices.SortFunc(ms, func(a, b match) int {
		return cmp.Or(cmp.Compare(a.start, b.start), cmp.Compare(b.end, a.end), cmp.Compare(a.link, b.link))
	})
	var out Text
	pos := 0
	for _, m := range ms {
		if m.start < pos {
			continue
		}
		if m.start > pos {
			out = append(out, Segment{Text: s[pos:m.start]})
		}
		lk := l.links[m.link]
		url := string(lk.re.ExpandString(nil, lk.url, s, m.idx))
		out = append(out, Segment{Text: s[m.start:m.end], URL: url})
		pos = m.end
	}
	if pos < len(s) {
		out = append(out, Segment{Text: s[pos:]})
	}
	return out
}
