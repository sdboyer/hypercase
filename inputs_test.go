package hypercase

import (
	"io/fs"
	"reflect"
	"testing"

	"github.com/sdboyer/hypercase/decl"
	"github.com/sdboyer/hypercase/internal/erase"
	"github.com/sdboyer/jenny"
)

// TestStated: what an Inputs states is told by its value alone, never by
// comparing functions: nil and a nil InputsFunc state nothing, an
// InputsFunc a map, each constant itself, and a value of the constants'
// kind that is neither, as arithmetic on them makes, is another.
func TestStated(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   decl.Inputs
		want statement
	}{
		{"nil", nil, statesNone},
		{"a nil InputsFunc", decl.InputsFunc(nil), statesNone},
		{"an InputsFunc", decl.InputsFunc(func(fs.FS, string) ([]decl.Range, error) { return nil, nil }), statesFunc},
		{"AllInputs", decl.AllInputs, statesAll},
		{"FromPositions", decl.FromPositions, statesPositions},
		{"AllInputs+2", decl.AllInputs + 2, statesOther},
		{"AllInputs-1", decl.AllInputs - 1, statesOther},
	} {
		if got := stated(tt.in); got != tt.want {
			t.Errorf("%s states %d, want %d", tt.name, got, tt.want)
		}
	}
}

// TestLiftInputsRoundTrip: a lift's output and product declarations reach
// the framework through its erased form, and each keeps its Inputs: an
// InputsFunc that the framework calls as the lift declared it, AllInputs,
// FromPositions, and none.
func TestLiftInputsRoundTrip(t *testing.T) {
	type in struct{ fs.FS }
	type out struct{ fs.FS }
	mapped := decl.InputsFunc(func(_ fs.FS, p string) ([]decl.Range, error) {
		return []decl.Range{{Path: "src/" + p, Start: 1, End: 2}}, nil
	})
	l := decl.Lift[in, out]{
		Name: "l",
		Outputs: []decl.Output{
			{Pattern: "error.txt", Description: "Why.", Inputs: decl.FromPositions},
			{Pattern: "stderr.txt", Description: "Noise.", Inputs: decl.AllInputs},
			{Pattern: "*.txt", Description: "Neither."},
		},
		Product: []decl.Output{
			{Pattern: "t/*", Description: "Mapped.", Inputs: mapped},
			{Pattern: "u/*", Description: "Whole.", Inputs: decl.AllInputs},
			{Pattern: "v/*", Description: "Neither.", Inputs: decl.InputsFunc(nil)},
		},
		Jenny: jenny.For[in]().New("l", func(in) (jenny.Files, error) { return nil, nil }),
	}
	for _, owned := range []bool{true, false} {
		e, ok := erase.Of(l)
		if !ok {
			t.Fatal("the lift has no erased form")
		}
		got := newLift(e.(*erase.Lift), owned)
		check := func(what string, got, want []decl.Output) {
			if len(got) != len(want) {
				t.Fatalf("owned %v: %s: %d declarations, want %d", owned, what, len(got), len(want))
			}
			for i := range want {
				if got[i].Pattern != want[i].Pattern || got[i].Description != want[i].Description || stated(got[i].Inputs) != stated(want[i].Inputs) {
					t.Errorf("owned %v: %s %d is %q, %q, stating %d, want %q, %q, stating %d", owned, what, i, got[i].Pattern, got[i].Description, stated(got[i].Inputs), want[i].Pattern, want[i].Description, stated(want[i].Inputs))
				}
			}
		}
		check("output", got.outputs, l.Outputs)
		check("product file", got.product, l.Product)
		f, ok := got.product[0].Inputs.(decl.InputsFunc)
		if !ok {
			t.Fatalf("owned %v: the mapped product file's Inputs is a %T", owned, got.product[0].Inputs)
		}
		rs, err := f(nil, "t/a")
		if want := []decl.Range{{Path: "src/t/a", Start: 1, End: 2}}; err != nil || !reflect.DeepEqual(rs, want) {
			t.Errorf("owned %v: the mapped product file's Inputs gives %v, %v, want %v", owned, rs, err, want)
		}
	}
}
