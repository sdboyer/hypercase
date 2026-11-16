package hypercase_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The doc golden. A code block in a Markdown doc can name the file it shows
// with a marker, an HTML comment on the line directly before the block's
// opening fence:
//
//	<!-- source: walkthrough/sheets/step01/evaluate_test.go -->
//	<!-- source: walkthrough/sheets/step02/evaluate_test.go#loop -->
//
// The file is under walkthrough/. The second form names a region of a Go
// file: the lines between the delimiter lines "// doc:begin loop" and
// "// doc:end loop". A block never shows a delimiter line, its own or another
// region's. Any other file is shown whole, with nothing taken out. The
// block's contents are the file's bytes, ending with a newline, each line
// indented as the block's opening fence is. A Go file shown whole that starts
// with a comment naming a path under walkthrough/ must name itself. The doc
// and its sources have LF line endings. A block with no marker is not
// checked.

const (
	docUpdateVar = "HYPERCASE_UPDATE_GOLDEN"
	docAdvice    = "If the change is intended, run HYPERCASE_UPDATE_GOLDEN=1 go test ./... and read the diff before committing."
)

var (
	// docMarkerStart is looser than docMarker, so that a near miss, such as
	// "Source:" or a missing colon, is reported rather than read as prose.
	docMarkerStart = regexp.MustCompile(`(?i)^ *<!--\s*source\b`)
	docMarker      = regexp.MustCompile(`^ *<!-- source: ([^\s#]+)(?:#([A-Za-z0-9_.-]+))? -->$`)
	docDelimStart  = regexp.MustCompile(`^//\s*doc:(?:begin|end)\b`)
	docDelim       = regexp.MustCompile(`^// doc:(begin|end) ([A-Za-z0-9_.-]+)$`)
	docHeader      = regexp.MustCompile(`^// (walkthrough/[^\s:]+)`)
)

// TestWalkthrough checks that each marked block of WALKTHROUGH.md shows its
// source under walkthrough/ byte for byte. With HYPERCASE_UPDATE_GOLDEN=1, it
// rewrites the blocks that differ.
func TestWalkthrough(t *testing.T) {
	update, err := docUpdateMode(os.Getenv(docUpdateVar))
	if err != nil {
		t.Fatal(err)
	}
	blocks, problems := docGolden(".", "WALKTHROUGH.md", update)
	for _, p := range problems {
		t.Error(p)
	}
	if blocks == 0 {
		t.Error("WALKTHROUGH.md has no marked blocks, so nothing in it is checked")
	}
}

// docUpdateMode reads the update variable's value: unset or empty checks the
// doc, 1 updates it, and anything else is refused, so that a value such as
// "true" can't pass for a check.
func docUpdateMode(v string) (bool, error) {
	switch v {
	case "":
		return false, nil
	case "1":
		return true, nil
	}
	return false, fmt.Errorf("%s is %q; set it to 1 to update the walkthrough's blocks, or leave it unset", docUpdateVar, v)
}

type docProblem struct {
	msg     string
	differs bool // the block differs from its source, which an update fixes
}

// docGolden checks the marked blocks of the doc at root/name against their
// sources under root, and returns how many marked blocks it has and a message
// for each problem, in doc order. With update, it rewrites each block that
// differs instead of reporting it.
func docGolden(root, name string, update bool) (blocks int, problems []string) {
	path := filepath.Join(root, name)
	doc, err := os.ReadFile(path)
	if err != nil {
		return 0, []string{err.Error()}
	}
	out, blocks, found := docRegenerate(os.DirFS(root), name, doc)
	for _, p := range found {
		if !update || !p.differs {
			problems = append(problems, p.msg)
		}
	}
	if update && !bytes.Equal(out, doc) {
		info, err := os.Stat(path)
		if err == nil {
			err = os.WriteFile(path, out, info.Mode().Perm())
		}
		if err != nil {
			problems = append(problems, err.Error())
		}
	}
	return blocks, problems
}

// docRegenerate returns doc with each marked block's contents regenerated
// from src, how many marked blocks it has, and its problems in doc order. A
// block it can't regenerate it leaves as it is.
func docRegenerate(src fs.FS, name string, doc []byte) (out []byte, blocks int, problems []docProblem) {
	if bytes.Contains(doc, []byte("\r\n")) {
		return doc, 0, []docProblem{{msg: name + " has CRLF line endings, and the doc golden reads LF only; check it out with LF"}}
	}
	lines := docLines(doc)
	done := 0 // the bytes of doc already copied to out
	for i := 0; i < len(lines); i++ {
		line := lines[i].trimmed()
		if !docMarkerStart.MatchString(line) {
			if indent, fence, ok := docOpenFence(line); ok {
				end, closed := docCloseFence(lines, i+1, indent, fence)
				if !closed {
					break // the block runs to the end of the doc
				}
				i = end
			}
			continue
		}

		fail := func(format string, args ...any) {
			problems = append(problems, docProblem{msg: fmt.Sprintf("%s:%d: ", name, i+1) + fmt.Sprintf(format, args...)})
		}
		m := docMarker.FindStringSubmatch(line)
		if m == nil {
			fail("malformed marker; want <!-- source: walkthrough/<path> --> or <!-- source: walkthrough/<path>#<region> -->")
			continue
		}
		source, region := m[1], m[2]
		if !fs.ValidPath(source) || !strings.HasPrefix(source, "walkthrough/") {
			fail("the marker names %s, which is not a file under walkthrough/", source)
			continue
		}
		indent, fence, ok := docOpenFence(lines[i+1].trimmed())
		if !ok {
			fail("the marker is not directly followed by a code block")
			continue
		}
		end, closed := docCloseFence(lines, i+2, indent, fence)
		if !closed {
			fail("its code block never closes")
			break
		}
		blocks++
		want, err := docSource(src, source, region, indent, fence)
		if err != nil {
			fail("%v", err)
			i = end
			continue
		}
		start, stop := lines[i+2].off, lines[end].off
		if got := doc[start:stop]; !bytes.Equal(got, want) {
			k, gotLine, wantLine := docFirstDiff(got, want)
			problems = append(problems, docProblem{differs: true, msg: fmt.Sprintf(
				"%s:%d: the block differs from %s, first at line %d:\n\tdoc:    %s\n\tsource: %s\n%s",
				name, i+1, docSourceName(source, region), i+3+k, gotLine, wantLine, docAdvice)})
			out = append(out, doc[done:start]...)
			out = append(out, want...)
			done = stop
		}
		i = end
	}
	return append(out, doc[done:]...), blocks, problems
}

type docLine struct {
	off  int
	text []byte // with its newline, if it has one
}

func (l docLine) trimmed() string { return string(bytes.TrimSuffix(l.text, []byte("\n"))) }

// docLines splits data into lines, and adds an empty last line at its end, so
// that lines[i].off is where line i starts, for every line and for the end.
func docLines(data []byte) []docLine {
	var lines []docLine
	for off := 0; off < len(data); {
		n := bytes.IndexByte(data[off:], '\n') + 1
		if n == 0 {
			n = len(data) - off
		}
		lines = append(lines, docLine{off, data[off : off+n]})
		off += n
	}
	return append(lines, docLine{len(data), nil})
}

// docOpenFence reports whether line opens a fenced code block, as CommonMark
// has it outside a container: three or more backticks or tildes, indented by
// at most three spaces, where a backtick fence's info string has no backtick.
// The doc golden doesn't follow containers, so a fence a list indents by four
// or more is not one to it.
func docOpenFence(line string) (indent, fence string, ok bool) {
	rest := strings.TrimLeft(line, " ")
	if len(line)-len(rest) > 3 || rest == "" || rest[0] != '`' && rest[0] != '~' {
		return "", "", false
	}
	n := len(rest) - len(strings.TrimLeft(rest, rest[:1]))
	if n < 3 || rest[0] == '`' && strings.Contains(rest[n:], "`") {
		return "", "", false
	}
	return line[:len(line)-len(rest)], rest[:n], true
}

// docCloses reports whether line closes a block opened by fence, indented by
// indent: as many of fence's character or more, indented by at most three
// spaces more than the fence, and nothing after but spaces and tabs.
func docCloses(line, indent, fence string) bool {
	rest := strings.TrimLeft(line, " ")
	if len(line)-len(rest) > len(indent)+3 {
		return false
	}
	n := len(rest) - len(strings.TrimLeft(rest, fence[:1]))
	return n >= len(fence) && strings.Trim(rest[n:], " \t") == ""
}

// docCloseFence returns the line, from line from on, that closes a block
// opened by fence, indented by indent.
func docCloseFence(lines []docLine, from int, indent, fence string) (int, bool) {
	for j := from; j < len(lines)-1; j++ {
		if docCloses(lines[j].trimmed(), indent, fence) {
			return j, true
		}
	}
	return 0, false
}

func docSourceName(source, region string) string {
	if region == "" {
		return source
	}
	return source + "#" + region
}

// docSource returns what a block opened by fence, indented by indent, holds
// when it shows region of source, or the whole of it if region is "".
func docSource(src fs.FS, source, region, indent, fence string) ([]byte, error) {
	data, err := fs.ReadFile(src, source)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s doesn't exist", source)
	}
	if err != nil {
		return nil, fmt.Errorf("can't read %s: %v", source, err)
	}
	if bytes.Contains(data, []byte("\r\n")) {
		return nil, fmt.Errorf("%s has CRLF line endings, and the doc golden reads LF only; check it out with LF", source)
	}
	isGo := strings.HasSuffix(source, ".go")
	if region != "" && !isGo {
		return nil, fmt.Errorf("%s is not a Go file, so it has no regions", source)
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	lines := docLines(data)
	lines = lines[:len(lines)-1]
	if isGo && region == "" && len(lines) > 0 {
		if m := docHeader.FindStringSubmatch(lines[0].trimmed()); m != nil && m[1] != source {
			return nil, fmt.Errorf("%s:1 names %s; the file is %s", source, m[1], source)
		}
	}

	// Every delimiter line, and the lines each region spans.
	delim := make([]bool, len(lines))
	type span struct{ begin, end int }
	regions := map[string]*span{}
	var open []string
	for j, l := range lines {
		text := strings.Trim(l.trimmed(), " \t")
		if !isGo || !docDelimStart.MatchString(text) {
			continue
		}
		at := fmt.Sprintf("%s:%d: ", source, j+1)
		m := docDelim.FindStringSubmatch(text)
		if m == nil {
			return nil, errors.New(at + "malformed region delimiter; want // doc:begin <name> or // doc:end <name>")
		}
		delim[j] = true
		s := regions[m[2]]
		switch {
		case m[1] == "begin" && s != nil:
			return nil, fmt.Errorf("%sregion %q begins twice", at, m[2])
		case m[1] == "begin":
			regions[m[2]] = &span{j, -1}
			open = append(open, m[2])
		case s == nil || s.end >= 0:
			return nil, fmt.Errorf("%sregion %q ends, but doesn't begin before it", at, m[2])
		default:
			s.end = j
			for k, name := range open {
				if name == m[2] {
					open = append(open[:k], open[k+1:]...)
					break
				}
			}
		}
	}
	if len(open) > 0 {
		return nil, fmt.Errorf("%s:%d: region %q never ends", source, regions[open[0]].begin+1, open[0])
	}

	from, to := 0, len(lines)
	if region != "" {
		s := regions[region]
		if s == nil {
			return nil, fmt.Errorf("%s has no region %q", source, region)
		}
		from, to = s.begin+1, s.end
	}
	var out []byte
	for j := from; j < to; j++ {
		if delim[j] {
			continue
		}
		// Indentation is all spaces, so a line closes the indented block
		// exactly when it would close the block unindented.
		if docCloses(lines[j].trimmed(), "", fence) {
			return nil, fmt.Errorf("%s:%d would close the block; give the block a longer fence", source, j+1)
		}
		if len(lines[j].text) > 1 {
			out = append(out, indent...)
		}
		out = append(out, lines[j].text...)
	}
	return out, nil
}

// docFirstDiff returns the first line, counted from 0, at which got and want
// differ, and each one's line there, quoted, or a note that it has ended.
func docFirstDiff(got, want []byte) (int, string, string) {
	g, w := docLines(got), docLines(want)
	for k := 0; ; k++ {
		gl, wl := "(the block ends)", "(the source ends)"
		if k < len(g)-1 {
			gl = fmt.Sprintf("%q", g[k].text)
		}
		if k < len(w)-1 {
			wl = fmt.Sprintf("%q", w[k].text)
		}
		if gl != wl {
			return k, gl, wl
		}
	}
}
