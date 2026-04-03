package runner

import (
	"context"
	"os"
	"testing"
)

func TestWorkspace_CreateAndRemove(t *testing.T) {
	if os.Getenv("CICADA_TEST_DOCKER") == "" {
		t.Skip("set CICADA_TEST_DOCKER=1 to run Docker integration tests")
	}

	ctx := context.Background()
	volumeName, err := CreateWorkspaceVolume(ctx, "cicada-test-ws-"+t.Name())
	if err != nil {
		t.Fatalf("CreateWorkspaceVolume() error = %v", err)
	}
	t.Cleanup(func() {
		if err := RemoveWorkspaceVolume(ctx, volumeName); err != nil {
			t.Logf("cleanup RemoveWorkspaceVolume() error = %v", err)
		}
	})

	if volumeName == "" {
		t.Error("expected non-empty volume name")
	}
}
