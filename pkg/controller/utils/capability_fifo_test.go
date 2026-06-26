//go:build unix

/*
 Copyright 2026 The KubeVela Authors.

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

package utils

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestReadTerraformConfigFromDirNonRegular guards the non-regular-file branch
// of the GHSA-fmgp-q6jx-gg3x fix: a non-symlink irregular file (a FIFO here)
// must be refused before any read, since reading it could block or stream
// without bound. This branch is what catches a non-symlink path to an
// unbounded source, complementing the symlink rejection.
func TestReadTerraformConfigFromDirNonRegular(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "variables.tf")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Skipf("mkfifo unsupported on this platform: %v", err)
	}
	defer func() { _ = os.Remove(fifo) }()

	_, err := readTerraformConfigFromDir(dir, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "non-regular")
}
