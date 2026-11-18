package hypercase

import (
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// TestJoinedClaims: a claim's sentence that goes on from advice ending
// with a comma reads in its joined form, which keeps the update command,
// and every name, as written: only a word that opens it, which is neither,
// is lower-cased. Each field of the claims, set alone, is rendered on its
// own and after such advice, under an update variable in capitals and one
// in lower case, so that a sentence that opens with the command is among
// them.
func TestJoinedClaims(t *testing.T) {
	const advice = "If not,"
	for _, v := range []string{"FIXTURE_UPDATE_GOLDEN", "fixture_update"} {
		st := textStaging(v)
		u := st.r.updateCmd()
		for _, one := range claimsAlone(t) {
			own := st.text(note{}, closing{claims: one.cl})
			joined := st.text(note{}, closing{advice: advice, claims: one.cl})
			if got, want := strings.Count(joined, u), strings.Count(own, u); got != want {
				t.Errorf("%s, under %s: the closing names the update command %d times on its own, and %d times after %q:\n%s\n%s", one.field, v, want, got, advice, own, joined)
			}
			rest, ok := strings.CutPrefix(joined, advice+" ")
			if !ok {
				// The claim says nothing after the advice.
				continue
			}
			want := own
			if !strings.HasPrefix(own, u) {
				r, size := utf8.DecodeRuneInString(own)
				want = string(unicode.ToLower(r)) + own[size:]
			}
			if rest != want {
				t.Errorf("%s, under %s: after %q, the closing goes on\n%q\nwant\n%q", one.field, v, advice, rest, want)
			}
		}
	}
}

// textStaging is a staging that renders a closing's claims as a cell's
// note in a package with the update variable v: of the column col, in a
// variant of the transform tr, which the package doesn't own.
func textStaging(v string) *staging {
	return &staging{
		r:   &runner{updateVar: v},
		c:   &aCase{name: "corpus/case/transform/tr", prefix: "transform/tr/", tr: &transform{name: "tr"}},
		col: &column{name: "col"},
	}
}

// A claimAlone is a closing's claims with one field set.
type claimAlone struct {
	field string
	cl    claims
}

// claimsAlone returns, for each field of a closing's claims, the claims
// with that field alone set: a string to "x", a bool to true, and Does to
// each kind of fate, as fateWords words them, with the fields a fate's
// claims always hold beside it: a broken fate's Reruns and Unwritten name
// its lift.
func claimsAlone(t *testing.T) []claimAlone {
	t.Helper()
	var out []claimAlone
	typ := reflect.TypeFor[claims]()
	for f := range typ.Fields() {
		var values []reflect.Value
		switch {
		case f.Name == "Does":
			for _, verb := range loadSource(t).stringsUsedIn(t, "fateWords") {
				w := verb
				if verb != fateRuns {
					w += " x"
				}
				values = append(values, reflect.ValueOf(w))
			}
		case f.Type.Kind() == reflect.String:
			values = append(values, reflect.ValueOf("x"))
		case f.Type.Kind() == reflect.Bool:
			values = append(values, reflect.ValueOf(true))
		case f.Name == "OnlyAdds":
			// The sections of the framework's files the upgrade adds:
			// each alone, and all of them.
			var all []string
			for _, p := range ranFiles {
				values = append(values, reflect.ValueOf([]string{"x/" + p}))
				all = append(all, "x/"+p)
			}
			if len(all) > 1 {
				values = append(values, reflect.ValueOf(all))
			}
		default:
			t.Fatalf("a closing's claims hold %s, a %s, which claimsAlone can't set", f.Name, f.Type)
		}
		for _, v := range values {
			var cl claims
			reflect.ValueOf(&cl).Elem().FieldByIndex(f.Index).Set(v)
			name := f.Name
			if f.Name == "Does" {
				name += " " + v.String()
				if verb, lift, _ := strings.Cut(v.String(), " "); verb == fateBroken {
					cl.Reruns, cl.Unwritten = lift, lift
				}
			}
			out = append(out, claimAlone{name, cl})
		}
	}
	return out
}

// TestBrokenClaims: a broken fate's sentence names the lift Does names,
// which its Reruns and Unwritten name too, as the claims of every broken
// fate the package tells do, and says once whose outcome and product a
// normal run reads. Claims that name another lift in either are the
// package's bug, which does refuses to word.
func TestBrokenClaims(t *testing.T) {
	t.Parallel()
	st := textStaging("FIXTURE_UPDATE_GOLDEN")
	got := st.does(claims{Does: fateBroken + " l2", Reruns: "l2", Unwritten: "l2"})
	want := "FIXTURE_UPDATE_GOLDEN=1 go test ./... writes nothing for col: it reruns l2, whose outcome and product a normal run reads from the archive, and it writes nothing for the cell of the lift l2, on col's path."
	if got != want {
		t.Errorf("a broken fate's sentence is\n%q\nwant\n%q", got, want)
	}
	for _, cl := range []claims{
		{Does: fateBroken + " l2", Reruns: "l1", Unwritten: "l2"},
		{Does: fateBroken + " l2", Reruns: "l2", Unwritten: "l1"},
		{Does: fateBroken + " l2"},
	} {
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("does words the claims %+v, whose lifts disagree", cl)
				}
			}()
			st.does(cl)
		}()
	}
}
