package report

import (
	"fmt"
	"html/template"
	"io"
	"strings"

	"github.com/sdboyer/hypercase/internal/diff"
	"github.com/sdboyer/hypercase/internal/registry"
)

// WriteHTML writes the report as one static HTML page, which loads
// nothing: its style sheet is inline, and it has no scripts. It holds the
// problems the report found, the corpus checks whose findings changed, the
// ranked changes with each changed file's line diff, what changed in its
// reverse map and the input lines it comes from, then the matrix of each
// corpus at B, each case with the properties
// it states and each known gap on the cell it marks, then the descriptions
// of what the registries declare. Text that names an issue links to it.
func (r *Report) WriteHTML(w io.Writer) error {
	return page.Execute(w, r)
}

// A rankGroup is the changes of one rank, for the page.
type rankGroup struct {
	Rank    Rank
	Changes []Change
}

func groups(changes []Change) []rankGroup {
	var out []rankGroup
	for _, ch := range changes {
		if len(out) == 0 || out[len(out)-1].Rank != ch.Rank {
			out = append(out, rankGroup{Rank: ch.Rank})
		}
		out[len(out)-1].Changes = append(out[len(out)-1].Changes, ch)
	}
	return out
}

// A problemGroup is the problems at one revision, for the page.
type problemGroup struct {
	At       string
	Problems []Problem
}

func problemGroups(ps []Problem) []problemGroup {
	var out []problemGroup
	for _, p := range ps {
		if len(out) == 0 || out[len(out)-1].At != p.At {
			out = append(out, problemGroup{At: p.At})
		}
		out[len(out)-1].Problems = append(out[len(out)-1].Problems, p)
	}
	return out
}

// stateOf returns an outcome's state, its first word.
func stateOf(outcome string) string {
	state, _, _ := strings.Cut(outcome, " ")
	return state
}

func plain(t Text) string {
	var b strings.Builder
	for _, s := range t {
		b.WriteString(s.Text)
	}
	return b.String()
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

// diffLine returns a diff line as the page shows it, and the class that
// styles it.
func diffLine(l diff.Line) (class, text string) {
	switch l.Kind {
	case diff.Skip:
		return "skip", "  ..."
	case diff.Unaligned:
		text = fmt.Sprintf("  ... left out: %s before and %s after, which the diff can't align", lines(l.Old), lines(l.New))
		if l.OldNoNewline {
			text += "\n- \\ no final newline"
		}
		if l.NewNoNewline {
			text += "\n+ \\ no final newline"
		}
		return "unaligned", text
	case '+':
		class = "add"
	case '-':
		class = "del"
	default:
		class = "ctx"
	}
	text = string(l.Kind) + " " + l.Text
	if l.NoNewline {
		text += "\n" + string(l.Kind) + ` \ no final newline`
	}
	return class, text
}

// lines says how many lines there are.
func lines(n int) string {
	if n == 1 {
		return "1 line"
	}
	return fmt.Sprintf("%d lines", n)
}

// rangeClass returns the class that styles a line of a reverse map that
// one revision's map lacks, as a diff line's.
func rangeClass(l RangeLine) string {
	if l.Kind == '-' {
		return "del"
	}
	return "add"
}

// excerptClass returns the class that styles a line of an excerpt: as a
// diff line's, by whose ranges hold it, where both revisions' do, and as
// the line that stands for lines left out.
func excerptClass(l ExcerptLine) string {
	switch {
	case l.N == 0:
		return "skip"
	case l.Mark == "-":
		return "del"
	case l.Mark == "+":
		return "add"
	}
	return "ctx"
}

// excerptText returns a line of an excerpt as the page shows it: its mark,
// where it has one, its number, and its text, or, for a line that stands
// for others, how many lines of a range it leaves out, if any.
func excerptText(l ExcerptLine) string {
	switch {
	case l.N == 0 && l.Cut > 0:
		return fmt.Sprintf("  ... left out: %s past the first %d of a range", lines(l.Cut), MaxRangeLines)
	case l.N == 0:
		return "  ..."
	}
	mark := ""
	if l.Mark != "" {
		mark = l.Mark + " "
	}
	text := fmt.Sprintf("%s%4d  %s", mark, l.N, l.Text)
	if l.NoNewline {
		text += fmt.Sprintf("\n%s%4s  \\ no final newline", mark, "")
	}
	return text
}

// statement says where an output declaration's ranges come from, as its
// Inputs states, or is "" where its registry records no statement.
func statement(o OutputDescribed) string {
	switch o.Inputs {
	case registry.InputsFunc:
		return "Its ranges come from a function."
	case registry.InputsPositions:
		if o.Positions {
			return "Its ranges come from Positions, which the project supplies."
		}
		return "Its ranges come from Positions, which the project doesn't supply."
	case registry.InputsAll:
		return "It depends on all inputs."
	case registry.InputsNone:
		return "It states no source of ranges."
	}
	return ""
}

// cutNote says what MaxFileLines leaves out of a file's input lines.
func cutNote(c *Cut) string {
	s := fmt.Sprintf("The page shows the first %d lines the file's ranges hold, and leaves out %d more, of %d section", MaxFileLines, c.Lines, c.Sections)
	if c.Sections != 1 {
		s += "s"
	}
	if c.Diffs > 0 {
		s += fmt.Sprintf(", and what changed in %d of them", c.Diffs)
	}
	return s + "."
}

var page = template.Must(template.New("page").Funcs(template.FuncMap{
	"groups":        groups,
	"problemGroups": problemGroups,
	"stateOf":       stateOf,
	"plain":         plain,
	"short":         short,
	"class":         Rank.class,
	"diffClass":     func(l diff.Line) string { c, _ := diffLine(l); return c },
	"diffText":      func(l diff.Line) string { _, t := diffLine(l); return t },
	"howClass":      func(how string) string { return strings.ReplaceAll(how, " ", "-") },
	"rangesOnly":    func(f FileChange) bool { return f.How == RangesOnly },
	"rangeClass":    rangeClass,
	"rangeText":     func(l RangeLine) string { return string(l.Kind) + " " + l.Text },
	"excerptClass":  excerptClass,
	"excerptText":   excerptText,
	"cutNote":       cutNote,
	"statement":     statement,
}).Parse(pageTemplate))

const pageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>hypercase report: {{.A.Name}} to {{.B.Name}}</title>
<style>` + pageStyle + `</style>
</head>
<body>
<header>
<h1>hypercase report</h1>
<p class="revs"><span class="rev">{{.A.Name}}</span> <code>{{short .A.Commit}}</code> to <span class="rev">{{.B.Name}}</span> <code>{{short .B.Commit}}</code>, over the corpus root <code>{{.Root}}</code></p>
</header>
<main>
{{- if .Problems}}
<section id="problems">
<h2>Problems <span class="count">{{len .Problems}}</span></h2>
{{- range problemGroups .Problems}}
<h3>At <span class="rev">{{.At}}</span></h3>
<ul class="problems">
{{- range .Problems}}
<li>{{if .Where}}<code>{{.Where}}</code>: {{end}}{{.Msg}}</li>
{{- end}}
</ul>
{{- end}}
</section>
{{- end}}
<section id="changes">
<h2>Changes</h2>
{{- with .Unranged}}
<p class="note">The newest registry at <span class="rev">{{.At}}</span> is of format {{.Format}}, from before inputs files, so the report shows no ranges at <span class="rev">{{.At}}</span>, and ranks no cell on its inputs file.</p>
{{- end}}
{{- if .Findings}}
<h3 class="rank findings-changed">findings changed <span class="count">{{len .Findings}}</span></h3>
<ul class="changes">
{{- range .Findings}}
<li class="change findings-changed">
<div class="line"><span class="pkg">{{.Package}}</span> <span class="check">{{.Check}}</span>{{with .Description}}: {{template "text" .}}{{end}}</div>
<pre class="findings">{{range .Disappeared}}<span class="del">{{print "- " .}}</span>{{end}}{{range .Appeared}}<span class="add">{{print "+ " .}}</span>{{end}}</pre>
</li>
{{- end}}
</ul>
{{- end}}
{{- if not .Changes}}
<p class="none">No cell changed.</p>
{{- end}}
{{- range groups .Changes}}
<h3 class="rank {{class .Rank}}">{{.Rank}} <span class="count">{{len .Changes}}</span></h3>
<ul class="changes">
{{- range .Changes}}
<li class="change {{class .Rank}}">
<div class="line"><span class="case">{{.Case}}</span> <span class="col">{{.Column}}</span>
{{- range $i, $o := .Outcomes}}{{if $i}} <span class="arrow">to</span>{{end}} <span class="outcome s-{{stateOf $o}}">{{$o}}</span>{{end}}
{{- template "props" .Properties}}</div>
{{- if .Files}}
<ul class="files">
{{- range .Files}}
<li class="{{howClass .How}}">{{if rangesOnly .}}<code>{{.Path}}</code> {{.How}}{{else}}<details><summary><code>{{.Path}}</code> {{.How}} <span class="stat">{{if .Unaligned}}partly unaligned{{else}}<span class="add">+{{.Added}}</span> <span class="del">-{{.Removed}}</span>{{end}}</span></summary>
<pre class="diff">{{range .Diff}}<span class="{{diffClass .}}">{{diffText .}}</span>{{end}}</pre></details>{{end}}
{{- if .Ranges}}
<pre class="ranges">{{range .Ranges}}<span class="{{rangeClass .}}">{{rangeText .}}</span>{{end}}</pre>
{{- end}}
{{- with .Inputs}}
<details class="inputs"><summary>input lines</summary>
{{- range .Maps}}{{with .}}{{if .All}}
<p class="map">At <span class="rev">{{.At}}</span>: all inputs.</p>
{{- else if not .Ranges}}
<p class="map">At <span class="rev">{{.At}}</span>: no ranges.</p>
{{- end}}{{if .Note}}
<p class="note">At <span class="rev">{{.At}}</span>: {{.Note}}</p>
{{- end}}{{end}}{{end}}
{{- range .Excerpts}}
<div class="excerpt"><code>{{.Section}}</code> at {{range $i, $r := .At}}{{if $i}} and {{end}}<span class="rev">{{$r}}</span>{{end}}
<pre class="lines">{{range .Lines}}<span class="{{excerptClass .}}">{{excerptText .}}</span>{{end}}</pre>
{{- if .Note}}
<p class="note">{{.Note}}</p>
{{- end}}
{{- if .Diff}}
<details><summary><code>{{.Section}}</code> changed <span class="stat">{{if .Unaligned}}partly unaligned{{else}}<span class="add">+{{.Added}}</span> <span class="del">-{{.Removed}}</span>{{end}}</span></summary>
<pre class="diff">{{range .Diff}}<span class="{{diffClass .}}">{{diffText .}}</span>{{end}}</pre></details>
{{- else if .DiffAt}}
<p class="note">What changed in <code>{{.Section}}</code> is shown under <code>{{.DiffAt}}</code>.</p>
{{- end}}</div>
{{- end}}
{{- with .Cut}}
<p class="note">{{cutNote .}}</p>
{{- end}}
</details>
{{- end}}</li>
{{- end}}
</ul>
{{- end}}
{{- if .Gap}}
<p class="gap"><span class="badge">known gap</span> {{template "text" .Gap}}</p>
{{- end}}
</li>
{{- end}}
</ul>
{{- end}}
</section>
{{- range .Matrices}}
<section class="matrix" id="corpus-{{.Corpus}}">
<h2>{{.Corpus}}</h2>
{{- if .Description}}
<p class="desc">{{template "text" .Description}}</p>
{{- end}}
<div class="scroll">
<table>
<thead>
<tr><th scope="col">case</th>{{range .Columns}}<th scope="col" title="{{plain .Description}}">{{.Name}}</th>{{end}}</tr>
</thead>
<tbody>
{{- range .Rows}}
<tr>
<th scope="row"><span class="case">{{.Case}}</span>{{template "props" .Properties}}
{{- if .Description}}<div class="desc">{{template "text" .Description}}</div>{{end}}</th>
{{- range .Cells}}
<td class="cell s-{{if .Present}}{{.State}}{{else}}empty{{end}}{{if .Changed}} {{class .Change}}{{end}}{{if .Frozen}} frozen{{end}}">
{{- if .Present}}<span class="outcome">{{.Outcome}}</span>{{end}}
{{- if .Frozen}} <span class="badge">frozen</span>{{end}}
{{- if .Changed}} <span class="badge">{{.Change}}</span>{{end}}
{{- if .Gap}}<div class="gap">{{template "text" .Gap}}</div>{{end -}}
</td>
{{- end}}
</tr>
{{- end}}
</tbody>
</table>
</div>
</section>
{{- end}}
<section id="descriptions">
<h2>Descriptions</h2>
{{- range .Corpora}}
<section class="corpus" id="describe-{{.Name}}">
<h3>{{.Name}}{{if .OnlyAtA}} <span class="badge">only at {{$.A.Name}}</span>{{end}}</h3>
{{- if .Description}}
<p>{{template "text" .Description}}</p>
{{- end}}
{{- if .Columns}}
<h4>Columns</h4>
<dl class="columns">
{{- range .Columns}}{{$only := .OnlyAtA}}
<dt>{{.Name}}{{if .OnlyAtA}} <span class="badge">only at {{$.A.Name}}</span>{{end}}</dt><dd>{{template "text" .Description}}
{{- if .Outputs}}<ul class="outputs">{{range .Outputs}}<li><code>{{.Name}}</code>{{if and .OnlyAtA (not $only)}} <span class="badge">only at {{$.A.Name}}</span>{{end}}: {{template "text" .Description}}{{with statement .}} <span class="statement">{{.}}</span>{{end}}</li>{{end}}</ul>{{end}}
{{- with .Classifier}}<p class="classifier">Its classifier, <code>{{.Name}}</code>{{if and .OnlyAtA (not $only)}} <span class="badge">only at {{$.A.Name}}</span>{{end}}: {{template "text" .Description}}</p>{{end -}}
</dd>
{{- end}}
</dl>
{{- end}}
{{- if .Transforms}}
<h4>Transforms</h4>
<dl class="transforms">
{{- range .Transforms}}
<dt>{{.Name}}{{if .OnlyAtA}} <span class="badge">only at {{$.A.Name}}</span>{{end}}</dt><dd>{{template "text" .Description}}
{{- if .Gives}}<p>Gives {{range $i, $p := .Gives}}{{if $i}}, {{end}}<span class="prop">{{$p}}</span>{{end}}.</p>{{end}}
{{- if .Keeps}}<p>Keeps {{range $i, $p := .Keeps}}{{if $i}}, {{end}}<span class="prop">{{$p}}</span>{{end}}.</p>{{end -}}
</dd>
{{- end}}
</dl>
{{- end}}
{{- if .Cases}}
<h4>Cases only at <span class="rev">{{$.A.Name}}</span></h4>
<dl class="cases">
{{- range .Cases}}
<dt><span class="case">{{.Name}}</span></dt><dd>{{template "text" .Description}}</dd>
{{- end}}
</dl>
{{- end}}
</section>
{{- end}}
{{- if .Properties}}
<section class="properties" id="describe-properties">
<h3>Properties</h3>
<dl class="properties">
{{- range .Properties}}
<dt>{{.Name}}{{if .OnlyAtA}} <span class="badge">only at {{$.A.Name}}</span>{{end}}</dt><dd>{{template "text" .Description}}</dd>
{{- end}}
</dl>
</section>
{{- end}}
</section>
</main>
</body>
</html>
{{define "text"}}{{range .}}{{if .URL}}<a href="{{.URL}}">{{.Text}}</a>{{else}}{{.Text}}{{end}}{{end}}{{end}}
{{- define "props"}}{{if .}} <span class="props">{{range .}}<span class="prop{{if .How}} {{.How}}{{end}}" title="{{plain .Description}}">{{.Name}}{{if .How}} <span class="how">{{.How}}</span>{{end}}</span>{{end}}</span>{{end}}{{end}}`

const pageStyle = `
:root {
  color-scheme: light dark;
  --bg: #fdfdfc; --fg: #1d1d1b; --muted: #6b6b66; --line: #deddd8; --head: #f3f2ee;
  --accepted: #e3f1e4; --rejected: #fbecd6; --panicked: #f8dcdc; --blocked: #e9e4f3; --skipped: #ecebe8;
  --changed: #c0392b; --appeared: #1f7a3f; --bytes: #9a6b00; --ranges: #2f7f86; --link: #1f5fa8;
  --add: #e3f1e4; --del: #f8dcdc;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #171716; --fg: #e8e7e3; --muted: #a09f99; --line: #3a3936; --head: #22221f;
    --accepted: #1f3a24; --rejected: #44341a; --panicked: #4a2222; --blocked: #322b45; --skipped: #2b2a27;
    --changed: #ff7b6b; --appeared: #6fd08f; --bytes: #e0b24a; --ranges: #6cc4c9; --link: #8cbcff;
    --add: #1f3a24; --del: #4a2222;
  }
}
* { box-sizing: border-box; }
body { margin: 0; padding: 0 16px 48px; background: var(--bg); color: var(--fg);
  font: 15px/1.45 system-ui, -apple-system, "Segoe UI", sans-serif; }
header, main { max-width: 1200px; margin: 0 auto; }
h1 { font-size: 1.5rem; margin: 24px 0 4px; }
h2 { font-size: 1.2rem; margin: 32px 0 8px; border-bottom: 1px solid var(--line); padding-bottom: 4px; }
h3 { font-size: 1rem; margin: 20px 0 6px; }
h4 { font-size: 0.95rem; margin: 12px 0 4px; color: var(--muted); }
code, pre, .case, .col, .pkg, .check, .outcome, .prop { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 0.9em; }
a { color: var(--link); }
.revs { color: var(--muted); margin: 0; }
.rev { font-weight: 600; }
.count { color: var(--muted); font-weight: normal; }
ul.problems { padding-left: 20px; }
ul.problems li { margin: 2px 0; overflow-wrap: anywhere; }
.rank.outcome-changed { color: var(--changed); }
.rank.appeared, .rank.disappeared { color: var(--appeared); }
.rank.bytes-changed { color: var(--bytes); }
.rank.ranges-changed { color: var(--ranges); }
ul.changes { list-style: none; padding: 0; margin: 0; }
li.change { padding: 6px 0; border-bottom: 1px solid var(--line); }
.line { overflow-wrap: anywhere; }
.col, .check { font-weight: 600; }
.outcome { padding: 1px 6px; border-radius: 4px; background: var(--head); }
.s-accepted .outcome, .outcome.s-accepted { background: var(--accepted); }
.s-rejected .outcome, .outcome.s-rejected { background: var(--rejected); }
.s-panicked .outcome, .outcome.s-panicked { background: var(--panicked); }
.s-blocked .outcome, .outcome.s-blocked { background: var(--blocked); }
.s-skipped .outcome, .outcome.s-skipped { background: var(--skipped); }
.outcome.s-unknown { font-style: italic; color: var(--muted); }
.arrow { color: var(--muted); }
ul.files { list-style: none; margin: 4px 0 0 20px; padding: 0; color: var(--muted); }
summary { cursor: pointer; }
.stat .add { color: var(--appeared); }
.stat .del { color: var(--changed); }
pre.diff, pre.ranges, pre.lines, pre.findings { margin: 4px 0 8px; padding: 6px 0; overflow-x: auto; color: var(--fg); background: var(--head); border-radius: 4px; }
pre.diff span, pre.ranges span, pre.lines span, pre.findings span { display: block; padding: 0 8px; white-space: pre; }
pre.diff .add, pre.ranges .add, pre.lines .add, pre.findings .add { background: var(--add); }
pre.diff .del, pre.ranges .del, pre.lines .del, pre.findings .del { background: var(--del); }
pre.diff .skip, pre.diff .unaligned, pre.lines .skip { color: var(--muted); }
details.inputs { margin: 2px 0 6px; }
details.inputs > summary, .excerpt { color: var(--muted); }
.excerpt { margin: 4px 0 0 16px; overflow-wrap: anywhere; }
.map { margin: 4px 0 0 16px; }
.props { margin-left: 6px; }
.prop { display: inline-block; margin: 0 4px 0 0; padding: 0 6px; border: 1px solid var(--line); border-radius: 10px; color: var(--muted); }
.prop.kept { border-style: dashed; }
.how { font-size: 0.85em; font-style: italic; }
.badge { display: inline-block; padding: 0 6px; border-radius: 10px; font-size: 0.8em; font-weight: normal; background: var(--head); color: var(--muted); border: 1px solid var(--line); }
.gap { margin: 4px 0 0; font-size: 0.9em; }
.scroll { overflow-x: auto; }
table { border-collapse: collapse; min-width: 100%; }
th, td { border: 1px solid var(--line); padding: 6px 8px; vertical-align: top; text-align: left; }
thead th { background: var(--head); white-space: nowrap; }
tbody th { font-weight: normal; min-width: 220px; }
.desc { color: var(--muted); font-size: 0.9em; margin-top: 2px; }
td.cell { min-width: 120px; }
td.s-accepted { background: var(--accepted); }
td.s-rejected { background: var(--rejected); }
td.s-panicked { background: var(--panicked); }
td.s-blocked { background: var(--blocked); }
td.s-skipped { background: var(--skipped); }
td.s-unknown .outcome { font-style: italic; color: var(--muted); }
td.cell .outcome { background: transparent; padding: 0; }
td.outcome-changed { box-shadow: inset 0 0 0 2px var(--changed); }
td.appeared { box-shadow: inset 0 0 0 2px var(--appeared); }
td.bytes-changed { box-shadow: inset 0 0 0 2px var(--bytes); }
td.ranges-changed { box-shadow: inset 0 0 0 2px var(--ranges); }
td.frozen { background-image: repeating-linear-gradient(135deg, transparent 0 8px, rgba(127,127,127,0.12) 8px 16px); }
dl { display: grid; grid-template-columns: max-content 1fr; gap: 4px 16px; margin: 4px 0 0; }
dt { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-weight: 600; }
dd { margin: 0; color: var(--muted); }
dd p, dd ul { margin: 2px 0 0; }
ul.outputs { padding-left: 18px; }
.statement { color: var(--muted); }
.none, .note { color: var(--muted); }
`
