package gitfs

// Blob reads an object as a tree's files do, by its hash, so a test can
// ask for one that isn't a blob.
func (r *Repo) Blob(hash string) ([]byte, error) { return r.blob(hash) }
