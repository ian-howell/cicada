package runner

import (
	"context"
	"fmt"

	"github.com/moby/moby/client"
)

// CreateWorkspaceVolume creates a named Docker volume for sharing files between step containers.
func CreateWorkspaceVolume(ctx context.Context, name string) (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	result, err := cli.VolumeCreate(ctx, client.VolumeCreateOptions{Name: name})
	if err != nil {
		return "", fmt.Errorf("create docker volume: %w", err)
	}
	return result.Volume.Name, nil
}

// RemoveWorkspaceVolume removes a Docker volume by name.
func RemoveWorkspaceVolume(ctx context.Context, name string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	if _, err := cli.VolumeRemove(ctx, name, client.VolumeRemoveOptions{Force: false}); err != nil {
		return fmt.Errorf("remove docker volume: %w", err)
	}
	return nil
}
