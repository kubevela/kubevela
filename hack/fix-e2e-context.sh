#!/usr/bin/env bash

# This script fixes syntax issues in e2e/commonContext.go
set -e

echo "===========> Fixing e2e/commonContext.go"

if [ -f "e2e/commonContext.go" ]; then
  # Create a backup
  cp e2e/commonContext.go e2e/commonContext.go.bak
  
  # Create a fixed version with proper syntax
  cat > e2e/commonContext.go.fixed << 'EOF'
/*
Copyright 2021 The KubeVela Authors.

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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

// Global variables for use in test functions
var (
	cli    string
	output string
	err    error
)

// Specify name of env
var EnvName = "default"

// GetClusterNameByEnv get cluster name from env
func GetClusterNameByEnv(envName string) string {
	output, _ := GetEnvByName(envName)
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "CLUSTER:") {
			return strings.TrimSpace(strings.Split(line, ":")[1])
		}
	}
	return ""
}

// GetEnvByName get env info by name
func GetEnvByName(envName string) (string, error) {
	cli = fmt.Sprintf("vela env ls %s", envName)
	return ExecCommand(cli)
}

// RemoveCluster remove cluster for test
func RemoveCluster(clusterName string) {
	cli = fmt.Sprintf("vela cluster remove %s", clusterName)
	_, err = ExecCommand(cli)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// EnvInitContext initialize test env
var EnvInitContext = func(context string, envName string) bool {
	return ginkgo.It(context+": should print env initiation successful message", func() {
		cli = fmt.Sprintf("vela env init %s", envName)
		output, err = ExecCommand(cli)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		expectedOutput := fmt.Sprintf("environment %s created", envName)
		gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
	})
}

// EnvShowContext show env
var EnvShowContext = func(context string, envName string) bool {
	return ginkgo.It(context+": should show environment", func() {
		cli = fmt.Sprintf("vela env ls %s", envName)
		output, err = ExecCommand(cli)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(output).To(gomega.ContainSubstring("NAME"))
		gomega.Expect(output).To(gomega.ContainSubstring(envName))
	})
}

// EnvSetContext set env
var EnvSetContext = func(context string, envName string) bool {
	return ginkgo.It(context+": should show environment set message", func() {
		cli = fmt.Sprintf("vela env sw %s", envName)
		output, err = ExecCommand(cli)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(output).To(gomega.ContainSubstring("switched to"))
	})
}

// EnvDeleteContext delete env
var EnvDeleteContext = func(context string, envName string) bool {
	return ginkgo.It(context+": should delete an environment", func() {
		cli = fmt.Sprintf("vela env delete %s", envName)
		output, err = ExecCommand(cli)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		gomega.Expect(output).To(gomega.ContainSubstring("deleted"))
	})
}

// DeleteEnvFunc delete env
var DeleteEnvFunc = func(context string, envName string) bool {
	return ginkgo.It(context+": should print env does not exist message", func() {
		cli = fmt.Sprintf("vela env delete %s", envName)
		_, err = ExecCommand(cli)
		gomega.Expect(err).To(gomega.HaveOccurred())
	})
}

// ExecCommand executes commands in shell and return the output
func ExecCommand(cmdString string) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd.exe", "/c", cmdString)
	} else {
		cmd = exec.Command("/bin/sh", "-c", cmdString)
	}
	cmd.Env = os.Environ()
	if runtime.GOOS == "windows" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("PATH=%s", filepath.Join(homedir(), ".vela", "bin")+string(os.PathListSeparator)+os.Getenv("PATH")))
	} else {
		cmd.Env = append(cmd.Env, fmt.Sprintf("PATH=%s", filepath.Join(homedir(), ".vela", "bin")+":"+os.Getenv("PATH")))
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func homedir() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	return home
}
EOF

  # Replace the file with the fixed version
  mv e2e/commonContext.go.fixed e2e/commonContext.go
  
  echo "===========> Fixed e2e/commonContext.go"
else
  echo "===========> e2e/commonContext.go not found"
fi
