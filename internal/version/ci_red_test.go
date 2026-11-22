package version

import "testing"

// TEMPORARY: proves the CI job turns red on a failing test (slice 2.1). Removed in the next commit.
func TestCIRedCheck(t *testing.T) {
	t.Fatal("deliberate failure to verify that CI turns red")
}
