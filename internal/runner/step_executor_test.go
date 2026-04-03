package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ianhomer/cicada/internal/model"
)

func TestRunStep_Success(t *testing.T) {
	if os.Getenv("CICADA_TEST_DOCKER") == "" {
		t.Skip("set CICADA_TEST_DOCKER=1 to run Docker integration tests")
	}

	ctx := context.Background()
	logDir := t.TempDir()

	volumeName, err := CreateWorkspaceVolume(ctx, "cicada-test-step-"+t.Name())
	if err != nil {
		t.Fatalf("CreateWorkspaceVolume() error = %v", err)
	}
	defer RemoveWorkspaceVolume(ctx, volumeName)

	step := model.Step{
		Name:     "hello",
		Image:    "alpine:latest",
		Commands: []string{"echo hello world"},
	}

	exitCode, err := RunStep(ctx, step, volumeName, t.TempDir(), logDir)
	if err != nil {
		t.Fatalf("RunStep() error = %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}

	logPath := filepath.Join(logDir, "hello.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected log file at %s: %v", logPath, err)
	}
	if !strings.Contains(string(data), "hello world") {
		t.Errorf("log does not contain 'hello world': %q", string(data))
	}
}

func TestRunStep_Failure(t *testing.T) {
	if os.Getenv("CICADA_TEST_DOCKER") == "" {
		t.Skip("set CICADA_TEST_DOCKER=1 to run Docker integration tests")
	}

	ctx := context.Background()
	logDir := t.TempDir()

	volumeName, err := CreateWorkspaceVolume(ctx, "cicada-test-stepfail-"+t.Name())
	if err != nil {
		t.Fatalf("CreateWorkspaceVolume() error = %v", err)
	}
	defer RemoveWorkspaceVolume(ctx, volumeName)

	step := model.Step{
		Name:     "fail",
		Image:    "alpine:latest",
		Commands: []string{"exit 1"},
	}

	exitCode, err := RunStep(ctx, step, volumeName, t.TempDir(), logDir)
	if err != nil {
		t.Fatalf("RunStep() error = %v", err)
	}
	if exitCode == 0 {
		t.Error("expected non-zero exit code for failing step")
	}
}
