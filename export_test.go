package hypercase

import "os"

// The self-tests run a child with reverseEnv set to 1 to run each case's
// cells in reverse column-name order.
const reverseEnv = "HYPERCASE_SELFTEST_REVERSE"

func init() {
	reverseCells = os.Getenv(reverseEnv) == "1"
}
