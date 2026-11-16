// Package hooks holds the points at which hypercase's tests watch its
// writes and make them fail, so a package's tests can reach those of the
// packages it calls. Only tests set them. A test that sets one doesn't run
// in parallel, and unsets it before it returns.
package hooks

// Modify, when set, is called at each step of archive.Modify: "lock", which
// comes just before the lock is taken, "locked", "read", "renamed" and
// "unlock", which comes just before the lock is released.
var Modify func(step string)

// AfterTemp, when set, runs in archive.WriteFile and archive.CreateFile
// once the temp file is written and synced, and before it is renamed over
// the file, or linked to the new file's name. An error it returns fails
// the write at that point.
var AfterTemp func(tmp string) error

// AfterMkdir, when set, runs in snapshot.Create once it has made or found
// the case's directory, and before it opens the case's temp file there.
var AfterMkdir func(dir string)

// ExtractFile, when set, runs in snapshot.Extract once each file is
// written. An error it returns fails the extract at that point.
var ExtractFile func(path string) error
