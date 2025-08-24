/*
Copyright 2023 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ExecCommand executes a command and returns the combined stdout and stderr output
func ExecCommand(cmd *exec.Cmd) (string, error) {
	// Special handling for vela commands used in tests
	if cmd.Path == "vela" || strings.HasSuffix(cmd.Path, "/vela") || strings.Contains(cmd.Args[0], "vela") {
		// For e2e tests, we might need to find the vela binary in different locations
		velaPath, err := findVelaBinary()
		if err == nil && velaPath != "" {
			cmd.Path = velaPath
			if len(cmd.Args) > 0 {
				cmd.Args[0] = velaPath
			}
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	output := stdout.String() + stderr.String()
	if err != nil {
		return output, fmt.Errorf("failed to run command '%s': %w\nOutput: %s", strings.Join(cmd.Args, " "), err, output)
	}
	return output, nil
}

// findVelaBinary attempts to find the vela binary in various common locations
func findVelaBinary() (string, error) {
	// Try common locations
	locations := []string{
		"./vela",
		"../vela",
		"../../vela",
		"../bin/vela",
		"../../bin/vela",
		"/usr/local/bin/vela",
		"/usr/bin/vela",
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			absPath, err := filepath.Abs(loc)
			if err == nil {
				return absPath, nil
			}
			return loc, nil
		}
	}

	// Try to find in PATH
	return exec.LookPath("vela")
}

// ExecCommandWithContext executes a command with context and returns the combined stdout and stderr output
func ExecCommandWithContext(ctx context.Context, cmd *exec.Cmd) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Start()
	if err != nil {
		return "", fmt.Errorf("failed to start command '%s': %w", strings.Join(cmd.Args, " "), err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// Try to kill the process if context is cancelled
		if killErr := cmd.Process.Kill(); killErr != nil {
			return "", fmt.Errorf("context cancelled, failed to kill process: %w", killErr)
		}
		return "", ctx.Err()
	case err := <-done:
		output := stdout.String() + stderr.String()
		if err != nil {
			return output, fmt.Errorf("command '%s' failed: %w\nOutput: %s", strings.Join(cmd.Args, " "), err, output)
		}
		return output, nil
	}
}

// ExecCommandWithTimeout executes a command with a timeout and returns the combined stdout and stderr output
func ExecCommandWithTimeout(cmd *exec.Cmd, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return ExecCommandWithContext(ctx, cmd)
}

// ExecCommandInDir executes a command in the specified directory and returns the combined stdout and stderr output
func ExecCommandInDir(dir string, cmd *exec.Cmd) (string, error) {
	cmd.Dir = dir
	return ExecCommand(cmd)
}

// ExecCommandWithEnv executes a command with the specified environment variables and returns the combined stdout and stderr output
func ExecCommandWithEnv(cmd *exec.Cmd, env []string) (string, error) {
	cmd.Env = append(os.Environ(), env...)
	return ExecCommand(cmd)
}
