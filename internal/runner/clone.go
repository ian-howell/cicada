package runner

import (
	"context"
	"fmt"
	"os/exec"
)

// CloneRepo clones the given repository URL into destDir and checks out the given commit SHA.
// Uses the system git binary.
func CloneRepo(ctx context.Context, cloneURL, commitSHA, destDir string) error {
	// Shallow clone to minimize bandwidth.
	cmd := exec.CommandContext(ctx, "git", "clone", "--no-checkout", cloneURL, destDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %w\n%s", err, out)
	}

	cmd = exec.CommandContext(ctx, "git", "-C", destDir, "checkout", commitSHA)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %w\n%s", commitSHA, err, out)
	}
	return nil
}
