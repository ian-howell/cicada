package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ian-howell/cicada/internal/model"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

// RunStep executes a single pipeline step in a Docker container.
// It streams stdout+stderr to a log file at logDir/<step.Name>.log.
// Returns the container exit code (non-zero means failure) and any system error.
func RunStep(ctx context.Context, step model.Step, volumeName, repoDir, logDir string) (int, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return -1, fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	cmd := buildCommand(step.Commands)

	env := make([]string, 0, len(step.Env))
	for k, v := range step.Env {
		env = append(env, k+"="+v)
	}

	resp, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:      step.Image,
			Cmd:        []string{"sh", "-c", cmd},
			Env:        env,
			WorkingDir: "/repo",
		},
		HostConfig: &container.HostConfig{
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeVolume,
					Source: volumeName,
					Target: "/workspace",
				},
				{
					Type:     mount.TypeBind,
					Source:   repoDir,
					Target:   "/repo",
					ReadOnly: true,
				},
			},
		},
	})
	if err != nil {
		return -1, fmt.Errorf("create container for step %q: %w", step.Name, err)
	}

	containerID := resp.ID
	defer func() {
		cli.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{Force: true})
	}()

	if _, err := cli.ContainerStart(ctx, containerID, client.ContainerStartOptions{}); err != nil {
		return -1, fmt.Errorf("start container for step %q: %w", step.Name, err)
	}

	// Stream logs to file.
	logFile := filepath.Join(logDir, step.Name+".log")
	if err := streamLogs(ctx, cli, containerID, logFile); err != nil {
		return -1, fmt.Errorf("stream logs for step %q: %w", step.Name, err)
	}

	// Wait for container to exit.
	waitResult := cli.ContainerWait(ctx, containerID, client.ContainerWaitOptions{
		Condition: container.WaitConditionNotRunning,
	})
	select {
	case err := <-waitResult.Error:
		if err != nil {
			return -1, fmt.Errorf("wait for container (step %q): %w", step.Name, err)
		}
		return -1, fmt.Errorf("unexpected nil error from container wait (step %q)", step.Name)
	case status := <-waitResult.Result:
		return int(status.StatusCode), nil
	}
}

func buildCommand(commands []string) string {
	return strings.Join(commands, " && ")
}

func streamLogs(ctx context.Context, cli *client.Client, containerID, logFile string) error {
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}
	defer f.Close()

	logs, err := cli.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return fmt.Errorf("attach logs: %w", err)
	}
	defer logs.Close()

	// Docker multiplexes stdout and stderr; strip the 8-byte header from each frame.
	// Frame format: [stream_type(1), 0, 0, 0, size_hi, size_b2, size_b1, size_lo][payload]
	buf := make([]byte, 8)
	for {
		_, err := io.ReadFull(logs, buf)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read log header: %w", err)
		}
		// buf[4:8] is the payload size (big-endian uint32).
		size := int(buf[4])<<24 | int(buf[5])<<16 | int(buf[6])<<8 | int(buf[7])
		if _, err := io.CopyN(f, logs, int64(size)); err != nil {
			return fmt.Errorf("copy log payload: %w", err)
		}
	}
	return nil
}
