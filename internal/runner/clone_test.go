package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCloneRepo(t *testing.T) {
	if os.Getenv("CICADA_TEST_DOCKER") == "" {
		t.Skip("set CICADA_TEST_DOCKER=1 to run Docker integration tests")
	}

	dir := t.TempDir()
	// Clone a small public repo at a known commit.
	err := CloneRepo(context.Background(), "https://github.com/git/git.git", "e83c5163316f89bfbde7d9ab23ca2e25604af290", dir)
	if err != nil {
		t.Fatalf("CloneRepo() error = %v", err)
	}

	// The very first git commit should have README.
	if _, err := os.Stat(filepath.Join(dir, "README")); err != nil {
		t.Errorf("expected README in cloned repo: %v", err)
	}
}
