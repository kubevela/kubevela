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

package kubeconfig

import (
	"os"
	"path/filepath"
	"testing"
)

const validKubeConfig = `apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://127.0.0.1:6443
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user: {}
`

// isolateFromAmbientConfig points HOME at an empty directory and clears the
// in-cluster service account hints, so the only kubeconfig the resolver can
// find is whatever the test sets explicitly.
func isolateFromAmbientConfig(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
}

func TestCheckWithoutKubeConfig(t *testing.T) {
	isolateFromAmbientConfig(t)
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "does-not-exist"))

	if err := Check(); err == nil {
		t.Fatal("expected an error when no kubeconfig can be resolved, got nil")
	}
	if Available() {
		t.Fatal("expected Available to report false when no kubeconfig can be resolved")
	}
}

func TestCheckWithKubeConfig(t *testing.T) {
	isolateFromAmbientConfig(t)
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(validKubeConfig), 0600); err != nil {
		t.Fatalf("failed to write kubeconfig: %v", err)
	}
	t.Setenv("KUBECONFIG", path)

	if err := Check(); err != nil {
		t.Fatalf("expected no error for a resolvable kubeconfig, got %v", err)
	}
	if !Available() {
		t.Fatal("expected Available to report true for a resolvable kubeconfig")
	}
}

// TestCheckDoesNotExit is the point of this package. The resolver underneath
// config.GetConfigOrDie terminates the process when it fails; Check must hand
// the failure back instead. If this ever regresses, the test binary dies here
// with no output rather than reporting a failure, which is exactly the symptom
// the package exists to prevent.
func TestCheckDoesNotExit(t *testing.T) {
	isolateFromAmbientConfig(t)
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "does-not-exist"))

	_ = Check()
	t.Log("survived Check with no resolvable kubeconfig")
}
