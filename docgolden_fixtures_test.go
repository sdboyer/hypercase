package hypercase_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docFixture writes files, by slash-separated path, under a new temporary
// directory, and returns the directory.
func docFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func readDoc(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "doc.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// checkDoc runs the doc golden on root/doc.md, and fails unless it reports
// exactly want.
func checkDoc(t *testing.T, root string, update bool, want ...string) {
	t.Helper()
	_, got := docGolden(root, "doc.md", update)
	if strings.Join(got, "\n--\n") != strings.Join(want, "\n--\n") {
		t.Errorf("update=%v: got problems:\n%s\nwant:\n%s", update, strings.Join(got, "\n--\n"), strings.Join(want, "\n--\n"))
	}
}

const docGoSource = "package a\n\nfunc g() {}\n"

func TestDocGoldenMatches(t *testing.T) {
	doc := "# Doc\n\n<!-- source: walkthrough/a/x.go -->\n```go\n" + docGoSource + "```\n\nProse.\n"
	root := docFixture(t, map[string]string{"doc.md": doc, "walkthrough/a/x.go": docGoSource})
	blocks, problems := docGolden(root, "doc.md", false)
	if blocks != 1 || len(problems) != 0 {
		t.Errorf("got %d blocks and problems %q, want 1 block and none", blocks, problems)
	}
	checkDoc(t, root, true)
	if got := readDoc(t, root); got != doc {
		t.Errorf("an update changed a doc that matches:\n%s", got)
	}
}

func TestDocGoldenDiffers(t *testing.T) {
	stale := "# Doc\n<!-- source: walkthrough/a/x.go -->\n```go\npackage a\n\nfunc f() {}\n```\n"
	root := docFixture(t, map[string]string{"doc.md": stale, "walkthrough/a/x.go": docGoSource})

	checkDoc(t, root, false, "doc.md:2: the block differs from walkthrough/a/x.go, first at line 6:\n"+
		"\tdoc:    \"func f() {}\\n\"\n"+
		"\tsource: \"func g() {}\\n\"\n"+
		"If the change is intended, run HYPERCASE_UPDATE_GOLDEN=1 go test ./... and read the diff before committing.")
	if got := readDoc(t, root); got != stale {
		t.Errorf("a check without update changed the doc:\n%s", got)
	}

	checkDoc(t, root, true)
	want := "# Doc\n<!-- source: walkthrough/a/x.go -->\n```go\n" + docGoSource + "```\n"
	if got := readDoc(t, root); got != want {
		t.Errorf("after the update:\ngot:\n%s\nwant:\n%s", got, want)
	}
	checkDoc(t, root, false)
}

func TestDocGoldenDiffersInLength(t *testing.T) {
	tests := []struct {
		name, block, want string
	}{
		{"block longer", docGoSource + "// more\n",
			"\tdoc:    \"// more\\n\"\n\tsource: (the source ends)\n"},
		{"source longer", "package a\n\n",
			"\tdoc:    (the block ends)\n\tsource: \"func g() {}\\n\"\n"},
		{"block empty", "",
			"\tdoc:    (the block ends)\n\tsource: \"package a\\n\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := "<!-- source: walkthrough/a/x.go -->\n```go\n" + tt.block + "```\n"
			root := docFixture(t, map[string]string{"doc.md": doc, "walkthrough/a/x.go": docGoSource})
			_, got := docGolden(root, "doc.md", false)
			if len(got) != 1 || !strings.Contains(got[0], tt.want) {
				t.Errorf("got %q, want one problem holding %q", got, tt.want)
			}
		})
	}
}

func TestDocGoldenRegion(t *testing.T) {
	source := "package a\n" +
		"\n" +
		"func f() {\n" +
		"\t// doc:begin loop\n" +
		"\tfor {\n" +
		"\t\t// doc:begin inner\n" +
		"\t\tx()\n" +
		"\t\t// doc:end inner\n" +
		"\t}\n" +
		"\t// doc:end loop\n" +
		"}\n" +
		"# doc:begin empty\n" +
		"# doc:end empty\n"
	block := func(src string) string { return "<!-- source: walkthrough/a/x.go" + src + " -->\n```go\nstale\n```\n" }
	doc := block("#loop") + block("#inner") + block("#empty") + block("")
	root := docFixture(t, map[string]string{"doc.md": doc, "walkthrough/a/x.go": source})

	checkDoc(t, root, true)
	want := "<!-- source: walkthrough/a/x.go#loop -->\n```go\n\tfor {\n\t\tx()\n\t}\n```\n" +
		"<!-- source: walkthrough/a/x.go#inner -->\n```go\n\t\tx()\n```\n" +
		"<!-- source: walkthrough/a/x.go#empty -->\n```go\n```\n" +
		"<!-- source: walkthrough/a/x.go -->\n```go\npackage a\n\nfunc f() {\n\tfor {\n\t\tx()\n\t}\n}\n```\n"
	if got := readDoc(t, root); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	checkDoc(t, root, false)
}

// An update rewrites the marked blocks and nothing else: prose, unmarked
// blocks and the doc's last line, without a newline, keep their bytes.
func TestDocGoldenUpdateKeepsTheRest(t *testing.T) {
	pieces := []string{
		"# Doc  \n\nProse with trailing spaces,  \n\ta tab, and <!-- a comment -->.\n\n",
		"<!-- source: walkthrough/a/x.go -->\n```go\n",
		"", // the first marked block
		"```\n\nAn unmarked block, stale and checked by nothing:\n\n```go\npackage stale\n```\n\n" +
			"A marker inside a block is part of the block:\n\n" +
			"````md\n<!-- source: walkthrough/a/none.go -->\n```go\n```\n````\n\n" +
			"~~~\n<!-- source: walkthrough/a/none.go -->\n~~~\n\n" +
			"- A list item, with its block indented:\n\n" +
			"  <!-- source: walkthrough/a/x.go -->\n  ```go\n",
		"", // the second marked block
		"  ```\n\n<!-- source: walkthrough/a/y.txt -->\n~~~~\n",
		"", // the third marked block
		"~~~~\nThe end, with no newline.",
	}
	stale := []string{"package old\n", "  package old\n", "old\n"}
	want := []string{docGoSource, "  package a\n\n  func g() {}\n", "```\nnot a fence here\n```\n"}
	join := func(blocks []string) string {
		p := append([]string(nil), pieces...)
		p[2], p[4], p[6] = blocks[0], blocks[1], blocks[2]
		return strings.Join(p, "")
	}
	root := docFixture(t, map[string]string{
		"doc.md":              join(stale),
		"walkthrough/a/x.go":  docGoSource,
		"walkthrough/a/y.txt": "```\nnot a fence here\n```\n",
	})

	blocks, problems := docGolden(root, "doc.md", false)
	if blocks != 3 || len(problems) != 3 {
		t.Fatalf("got %d blocks and problems:\n%s\nwant 3 blocks, each differing", blocks, strings.Join(problems, "\n--\n"))
	}
	checkDoc(t, root, true)
	if got, want := readDoc(t, root), join(want); got != want {
		t.Errorf("after the update:\ngot:\n%s\nwant:\n%s", got, want)
	}
	checkDoc(t, root, false)
}

func TestDocGoldenUnmarked(t *testing.T) {
	doc := "# Doc\n\n```go\nanything at all\n```\n\n```sh\ngo test ./...\n```\n\n" +
		"```\n<!-- source: walkthrough/none.go -->\n```go\n```\n"
	root := docFixture(t, map[string]string{"doc.md": doc})
	blocks, problems := docGolden(root, "doc.md", false)
	if blocks != 0 || len(problems) != 0 {
		t.Errorf("got %d blocks and problems %q, want none", blocks, problems)
	}
	checkDoc(t, root, true)
	if got := readDoc(t, root); got != doc {
		t.Errorf("an update changed a doc with no marked blocks:\n%s", got)
	}
}

// A source without a last newline is shown with one.
func TestDocGoldenNoFinalNewline(t *testing.T) {
	doc := "<!-- source: walkthrough/a/x.txt -->\n```\nA1 6\nB1 3\n```\n"
	root := docFixture(t, map[string]string{"doc.md": doc, "walkthrough/a/x.txt": "A1 6\nB1 3"})
	checkDoc(t, root, false)
}

// Problems come in doc order, and an update rewrites the blocks it can while
// reporting the ones it can't.
func TestDocGoldenSeveral(t *testing.T) {
	doc := "<!-- source: walkthrough/a/none.go -->\n```go\n```\n" +
		"<!-- source: walkthrough/a/x.go -->\n```go\nstale\n```\n" +
		"<!-- source: walkthrough/a/x.go#none -->\n```go\n```\n"
	root := docFixture(t, map[string]string{"doc.md": doc, "walkthrough/a/x.go": docGoSource})
	missing := "doc.md:1: walkthrough/a/none.go doesn't exist"
	region := `doc.md:8: walkthrough/a/x.go has no region "none"`
	_, got := docGolden(root, "doc.md", false)
	if len(got) != 3 || got[0] != missing || !strings.HasPrefix(got[1], "doc.md:4: the block differs") || got[2] != region {
		t.Errorf("got problems:\n%s", strings.Join(got, "\n--\n"))
	}
	checkDoc(t, root, true, missing, region)
	want := strings.Replace(doc, "stale\n", docGoSource, 1)
	if got := readDoc(t, root); got != want {
		t.Errorf("after the update:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestDocGoldenBroken(t *testing.T) {
	marked := func(src string) string { return "<!-- source: " + src + " -->\n```go\n```\n" }
	tests := []struct {
		name   string
		doc    string
		source string // walkthrough/a/x.go
		want   string
	}{
		{"missing file", marked("walkthrough/a/none.go"), docGoSource,
			"doc.md:1: walkthrough/a/none.go doesn't exist"},
		{"missing region", marked("walkthrough/a/x.go#loop"), docGoSource,
			`doc.md:1: walkthrough/a/x.go has no region "loop"`},
		{"outside walkthrough", marked("a/x.go"), docGoSource,
			"doc.md:1: the marker names a/x.go, which is not a file under walkthrough/"},
		{"out of the tree", marked("walkthrough/../go.mod"), docGoSource,
			"doc.md:1: the marker names walkthrough/../go.mod, which is not a file under walkthrough/"},
		{"no space", "<!-- source:walkthrough/a/x.go -->\n```go\n```\n", docGoSource,
			"doc.md:1: malformed marker; want <!-- source: walkthrough/<path> --> or <!-- source: walkthrough/<path>#<region> -->"},
		{"empty region", marked("walkthrough/a/x.go#"), docGoSource,
			"doc.md:1: malformed marker; want <!-- source: walkthrough/<path> --> or <!-- source: walkthrough/<path>#<region> -->"},
		{"no path", marked(""), docGoSource,
			"doc.md:1: malformed marker; want <!-- source: walkthrough/<path> --> or <!-- source: walkthrough/<path>#<region> -->"},
		{"blank line before the block", "<!-- source: walkthrough/a/x.go -->\n\n```go\n```\n", docGoSource,
			"doc.md:1: the marker is not directly followed by a code block"},
		{"two markers", "<!-- source: walkthrough/a/x.go -->\n" + marked("walkthrough/a/x.go"), "",
			"doc.md:1: the marker is not directly followed by a code block"},
		{"at the end", "Prose.\n<!-- source: walkthrough/a/x.go -->\n", docGoSource,
			"doc.md:2: the marker is not directly followed by a code block"},
		{"block never closes", "<!-- source: walkthrough/a/x.go -->\n```go\npackage a\n", docGoSource,
			"doc.md:1: its code block never closes"},
		{"region never ends", marked("walkthrough/a/x.go#loop"), "package a\n// doc:begin loop\n",
			`doc.md:1: walkthrough/a/x.go:2: region "loop" never ends`},
		{"region begins twice", marked("walkthrough/a/x.go"), "// doc:begin loop\n// doc:end loop\n// doc:begin loop\n",
			`doc.md:1: walkthrough/a/x.go:3: region "loop" begins twice`},
		{"region ends first", marked("walkthrough/a/x.go"), "// doc:end loop\n",
			`doc.md:1: walkthrough/a/x.go:1: region "loop" ends, but doesn't begin before it`},
		{"region ends twice", marked("walkthrough/a/x.go"), "// doc:begin loop\n// doc:end loop\n// doc:end loop\n",
			`doc.md:1: walkthrough/a/x.go:3: region "loop" ends, but doesn't begin before it`},
		{"delimiter without a name", marked("walkthrough/a/x.go"), "// doc:begin\n",
			"doc.md:1: walkthrough/a/x.go:1: malformed region delimiter; want // doc:begin <name> or // doc:end <name>"},
		{"delimiter with two names", marked("walkthrough/a/x.go"), "\t//  doc:end a b\n",
			"doc.md:1: walkthrough/a/x.go:1: malformed region delimiter; want // doc:begin <name> or // doc:end <name>"},
		{"source closes the fence", marked("walkthrough/a/x.go"), "a\n   ````  \nb\n",
			"doc.md:1: walkthrough/a/x.go:2 would close the block; give the block a longer fence"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := docFixture(t, map[string]string{"doc.md": tt.doc, "walkthrough/a/x.go": tt.source})
			checkDoc(t, root, false, tt.want)
			checkDoc(t, root, true, tt.want)
			if got := readDoc(t, root); got != tt.doc {
				t.Errorf("an update changed the doc:\n%s", got)
			}
		})
	}
}

// A line that doesn't close the block's fence is shown as it is.
func TestDocGoldenNotAFence(t *testing.T) {
	source := "```\n    ```\n```go\n``x\n"
	for _, fence := range []string{"````", "~~~"} {
		doc := "<!-- source: walkthrough/a/x.md -->\n" + fence + "md\n" + source + fence + "\n"
		root := docFixture(t, map[string]string{"doc.md": doc, "walkthrough/a/x.md": source})
		checkDoc(t, root, false)
	}
	doc := "<!-- source: walkthrough/a/x.md -->\n```md\n    ```\n```\n"
	root := docFixture(t, map[string]string{"doc.md": doc, "walkthrough/a/x.md": "    ```\n"})
	checkDoc(t, root, false)
}

// A marker naming something that can't be read fails, naming it. The reason
// after it is the operating system's.
func TestDocGoldenUnreadable(t *testing.T) {
	doc := "<!-- source: walkthrough/a -->\n```\n```\n"
	root := docFixture(t, map[string]string{"doc.md": doc, "walkthrough/a/x.go": docGoSource})
	for _, update := range []bool{false, true} {
		_, got := docGolden(root, "doc.md", update)
		if len(got) != 1 || !strings.HasPrefix(got[0], "doc.md:1: can't read walkthrough/a: ") {
			t.Errorf("update=%v: got %q, want one problem naming walkthrough/a", update, got)
		}
	}
	if got := readDoc(t, root); got != doc {
		t.Errorf("an update changed the doc:\n%s", got)
	}
}

func TestDocUpdateMode(t *testing.T) {
	tests := []struct {
		value  string
		update bool
		err    string
	}{
		{"", false, ""},
		{"1", true, ""},
		{"0", false, `HYPERCASE_UPDATE_GOLDEN is "0"; set it to 1 to update the walkthrough's blocks, or leave it unset`},
		{"true", false, `HYPERCASE_UPDATE_GOLDEN is "true"; set it to 1 to update the walkthrough's blocks, or leave it unset`},
	}
	for _, tt := range tests {
		update, err := docUpdateMode(tt.value)
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		if update != tt.update || msg != tt.err {
			t.Errorf("%q: got %v, %q; want %v, %q", tt.value, update, msg, tt.update, tt.err)
		}
	}
}
