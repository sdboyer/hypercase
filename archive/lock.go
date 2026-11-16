package archive

import (
	"bytes"
	"os"

	"github.com/rogpeppe/go-internal/lockedfile"
	"github.com/sdboyer/hypercase/internal/hooks"
)

// LockFile is the name of a corpus root's lock file, in the root's
// testdata/ directory. Every update under the root takes it around each
// read-modify-write of an archive. It is never renamed or removed, and git
// ignores it.
const LockFile = ".hypercase.lock"

// hook calls hooks.Modify, through which tests sequence the steps of
// [Modify].
func hook(step string) {
	if hooks.Modify != nil {
		hooks.Modify(step)
	}
}

// Modify is a read-modify-write of the archive at path, under the lock on
// the file at lockPath: it takes the lock, re-reads the archive, and calls
// fn with what it read. It writes the archive fn returns with [Write], by
// temp file and rename, and only then releases the lock. When fn returns
// nil, or an archive whose bytes are those read, nothing is written. An
// error from fn is returned, and nothing is written.
//
// The lock is an OS advisory lock, which the kernel releases when the
// process exits, however it exits. It is taken on a file of its own, never
// on the archive, because the rename replaces the archive's file: a waiter
// holding the old file would take a lock nobody else waits on.
func Modify(lockPath, path string, fn func(current *Archive) (*Archive, error)) error {
	unlock, err := lockedfile.MutexAt(lockPath).Lock()
	if err != nil {
		return err
	}
	defer func() {
		hook("unlock")
		unlock()
	}()
	hook("locked")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	hook("read")
	next, err := fn(Parse(data))
	if err != nil || next == nil {
		return err
	}
	if bytes.Equal(next.Format(), data) {
		return nil
	}
	if err := Write(path, next); err != nil {
		return err
	}
	hook("renamed")
	return nil
}
