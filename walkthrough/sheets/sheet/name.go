package sheet

import (
	"cmp"
	"strconv"
)

// A sheet's bounds: columns A to XFD, rows 1 to 1048576.
const (
	lastCol = 16384
	lastRow = 1048576
)

// pos is a cell's place: its column and row, both counted from 1.
type pos struct{ col, row int }

// parseName reads a cell name such as B12: one to three capital letters,
// then a row number with no leading zero, within a sheet's bounds.
func parseName(s string) (pos, bool) {
	i := 0
	for i < len(s) && 'A' <= s[i] && s[i] <= 'Z' {
		i++
	}
	if i == 0 || i > 3 || i == len(s) || s[i] == '0' || len(s)-i > 7 {
		return pos{}, false
	}
	var p pos
	for _, c := range []byte(s[:i]) {
		p.col = p.col*26 + int(c-'A') + 1
	}
	for _, c := range []byte(s[i:]) {
		if c < '0' || c > '9' {
			return pos{}, false
		}
		p.row = p.row*10 + int(c-'0')
	}
	if p.col > lastCol || p.row > lastRow {
		return pos{}, false
	}
	return p, true
}

// String returns the cell's name.
func (p pos) String() string {
	var col []byte
	for c := p.col; c > 0; c = (c - 1) / 26 {
		col = append(col, byte('A'+(c-1)%26))
	}
	for i, j := 0, len(col)-1; i < j; i, j = i+1, j-1 {
		col[i], col[j] = col[j], col[i]
	}
	return string(col) + strconv.Itoa(p.row)
}

// compare orders cells row by row, then column by column.
func (p pos) compare(q pos) int {
	if c := cmp.Compare(p.row, q.row); c != 0 {
		return c
	}
	return cmp.Compare(p.col, q.col)
}

// compareNames orders names in cell order. A string that is not a cell name
// sorts after every cell name, and among such strings, byte by byte.
func compareNames(a, b string) int {
	p, aok := parseName(a)
	q, bok := parseName(b)
	switch {
	case aok && bok:
		return p.compare(q)
	case aok:
		return -1
	case bok:
		return 1
	}
	return cmp.Compare(a, b)
}
