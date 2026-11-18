// Package hooks holds the points at which hypercase's tests watch its
// writes and make them fail, so a package's tests can reach those of the
// packages it calls. Only tests set them. A test that sets one doesn't run
// in parallel, and unsets it before it returns.
package hooks

// Modify, when set, is called at each step of archive.Modify: "lock", which
// comes just before the lock is taken, "locked", "read", "renamed" and
// "unlock", which comes just before the lock is released. It is called at
// each step of archive.RemoveFile too: "lock", "locked", "removed" and
// "unlock".
var Modify func(step string)

// AfterTemp, when set, runs in archive.WriteFile once the temp file is
// written and synced, and before it is renamed over the file. An error it
// returns fails the write at that point.
var AfterTemp func(tmp string) error
