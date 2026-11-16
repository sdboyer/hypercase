package report

import (
	"html/template"
	"io"
	"strings"
)

// WriteHTML writes the report as one static HTML page, which loads
// nothing: its style sheet is inline, and it has no scripts. It holds the
// ranked changes, then the matrix of each corpus at B, each case with the
// properties it states and each known gap on the cell it marks, linked to
// its issue.
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

// stateOf returns an outcome's state, or "none" for a cell with no outcome
// file.
func stateOf(outcome string) string {
	if outcome == "" || outcome == noOutcome {
		return "none"
	}
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

var page = template.Must(template.New("page").Funcs(template.FuncMap{
	"groups":  groups,
	"stateOf": stateOf,
	"plain":   plain,
	"short":   short,
	"class":   Rank.class,
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
<section id="changes">
<h2>Changes</h2>
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
<li class="{{.How}}"><code>{{.Path}}</code> {{.How}}</li>
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
<td class="cell s-{{if .Present}}{{stateOf .Outcome}}{{else}}empty{{end}}{{if .Changed}} {{class .Change}}{{end}}{{if .Frozen}} frozen{{end}}">
{{- if .Present}}<span class="outcome">{{if .Outcome}}{{.Outcome}}{{else}}(no outcome){{end}}</span>{{end}}
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
<dl class="columns">
{{- range .Columns}}
<dt>{{.Name}}</dt><dd>{{if .Description}}{{template "text" .Description}}{{else}}No registry records this column.{{end}}</dd>
{{- end}}
</dl>
</section>
{{- end}}
</main>
</body>
</html>
{{define "text"}}{{range .}}{{if .URL}}<a href="{{.URL}}">{{.Text}}</a>{{else}}{{.Text}}{{end}}{{end}}{{end}}
{{- define "props"}}{{if .}} <span class="props">{{range .}}<span class="prop" title="{{plain .Description}}">{{.Name}}</span>{{end}}</span>{{end}}{{end}}`

const pageStyle = `
:root {
  color-scheme: light dark;
  --bg: #fdfdfc; --fg: #1d1d1b; --muted: #6b6b66; --line: #deddd8; --head: #f3f2ee;
  --accepted: #e3f1e4; --rejected: #fbecd6; --panicked: #f8dcdc; --blocked: #e9e4f3; --skipped: #ecebe8; --none: #f6e9f0;
  --changed: #c0392b; --appeared: #1f7a3f; --bytes: #9a6b00; --link: #1f5fa8;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #171716; --fg: #e8e7e3; --muted: #a09f99; --line: #3a3936; --head: #22221f;
    --accepted: #1f3a24; --rejected: #44341a; --panicked: #4a2222; --blocked: #322b45; --skipped: #2b2a27; --none: #40283a;
    --changed: #ff7b6b; --appeared: #6fd08f; --bytes: #e0b24a; --link: #8cbcff;
  }
}
* { box-sizing: border-box; }
body { margin: 0; padding: 0 16px 48px; background: var(--bg); color: var(--fg);
  font: 15px/1.45 system-ui, -apple-system, "Segoe UI", sans-serif; }
header, main { max-width: 1200px; margin: 0 auto; }
h1 { font-size: 1.5rem; margin: 24px 0 4px; }
h2 { font-size: 1.2rem; margin: 32px 0 8px; border-bottom: 1px solid var(--line); padding-bottom: 4px; }
h3 { font-size: 1rem; margin: 20px 0 6px; }
code, .case, .col, .outcome, .prop { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 0.9em; }
a { color: var(--link); }
.revs { color: var(--muted); margin: 0; }
.rev { color: var(--fg); font-weight: 600; }
.count { color: var(--muted); font-weight: normal; }
.rank.outcome-changed { color: var(--changed); }
.rank.appeared, .rank.disappeared { color: var(--appeared); }
.rank.bytes-changed { color: var(--bytes); }
ul.changes { list-style: none; padding: 0; margin: 0; }
li.change { padding: 6px 0; border-bottom: 1px solid var(--line); }
.line { overflow-wrap: anywhere; }
.col { font-weight: 600; }
.outcome { padding: 1px 6px; border-radius: 4px; background: var(--head); }
.s-accepted .outcome, .outcome.s-accepted { background: var(--accepted); }
.s-rejected .outcome, .outcome.s-rejected { background: var(--rejected); }
.s-panicked .outcome, .outcome.s-panicked { background: var(--panicked); }
.s-blocked .outcome, .outcome.s-blocked { background: var(--blocked); }
.s-skipped .outcome, .outcome.s-skipped { background: var(--skipped); }
.s-none .outcome, .outcome.s-none { background: var(--none); }
.arrow { color: var(--muted); }
ul.files { margin: 4px 0 0 20px; padding: 0; color: var(--muted); }
.props { margin-left: 6px; }
.prop { display: inline-block; margin: 0 4px 0 0; padding: 0 6px; border: 1px solid var(--line); border-radius: 10px; color: var(--muted); }
.badge { display: inline-block; padding: 0 6px; border-radius: 10px; font-size: 0.8em; background: var(--head); color: var(--muted); border: 1px solid var(--line); }
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
td.s-none { background: var(--none); }
td.cell .outcome { background: transparent; padding: 0; }
td.outcome-changed { box-shadow: inset 0 0 0 2px var(--changed); }
td.appeared { box-shadow: inset 0 0 0 2px var(--appeared); }
td.bytes-changed { box-shadow: inset 0 0 0 2px var(--bytes); }
td.frozen { background-image: repeating-linear-gradient(135deg, transparent 0 8px, rgba(127,127,127,0.12) 8px 16px); }
dl.columns { display: grid; grid-template-columns: max-content 1fr; gap: 4px 16px; margin: 12px 0 0; }
dl.columns dt { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-weight: 600; }
dl.columns dd { margin: 0; color: var(--muted); }
.none { color: var(--muted); }
`
