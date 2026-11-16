// Package dirs indexes the directories that a set of file paths implies,
// for the read-only file systems that hold a known set of files: an
// archive's input view, and a revision's tree.
package dirs

import (
	"iter"
	"maps"
	"path"
	"slices"
)

// Index returns each directory the slash paths of files imply, with the
// names of its entries, sorted. The root, ".", is always a directory.
func Index(files iter.Seq[string]) map[string][]string {
	kids := map[string]map[string]bool{".": {}}
	for name := range files {
		for child := name; child != "."; child = path.Dir(child) {
			dir := path.Dir(child)
			set, known := kids[dir]
			if !known {
				set = map[string]bool{}
				kids[dir] = set
			}
			set[path.Base(child)] = true
			if known {
				break
			}
		}
	}
	index := make(map[string][]string, len(kids))
	for dir, set := range kids {
		index[dir] = slices.Sorted(maps.Keys(set))
	}
	return index
}
