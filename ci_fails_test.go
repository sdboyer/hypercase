package hypercase

import "testing"

// TestCIFails exists only to show that CI fails on a failing test. It is
// never merged.
func TestCIFails(t *testing.T) {
	t.Fatal("this test fails on purpose, to show CI goes red on both legs")
}
