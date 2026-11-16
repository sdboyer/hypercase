// walkthrough/sheets/step01/evaluate_test.go
package step01

import (
	"maps"
	"testing"

	"github.com/sdboyer/hypercase/walkthrough/sheets/sheet"
)

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name  string
		cells map[string]string
		want  sheet.Values
	}{
		{
			name:  "twice",
			cells: map[string]string{"A1": "=B1*2", "B1": "3"},
			want:  sheet.Values{"A1": 6, "B1": 3},
		},
		{
			name:  "prices",
			cells: map[string]string{"A1": "4", "B1": "6", "C1": "=A1+B1"},
			want:  sheet.Values{"A1": 4, "B1": 6, "C1": 10},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sheet.EvalCells(tt.cells)
			if err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
