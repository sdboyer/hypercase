// walkthrough/sheets/step02/evaluate_test.go
package step02

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name  string
		cells map[string]string
	}{
		{"twice", map[string]string{"A1": "=B1*2", "B1": "3"}},
		{"prices", map[string]string{"A1": "4", "B1": "6", "C1": "=A1+B1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := sheet.EvalCells(tt.cells)
			if err != nil {
				t.Fatal(err)
			}
			got := values.Format()
			golden := filepath.Join("testdata", tt.name+".golden")
			if *update {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s differs:\ngot:\n%s\nwant:\n%s\n"+
					"If the change is intended, run go test ./step02 -update and read the diff.",
					golden, got, want)
			}
		})
	}
}
