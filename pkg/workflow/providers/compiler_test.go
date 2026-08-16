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

package providers

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// childEnvVar marks the re-executed copy of this test binary that does the
// actual work. See TestDefaultCompilerWithoutKubeConfig.
const childEnvVar = "VELA_TEST_COMPILER_NO_KUBECONFIG_CHILD"

// TestDefaultCompilerWithoutKubeConfig pins the fix for the process exiting
// during compiler construction when no kubeconfig is present.
//
// Building the compiler used to reach config.GetConfigOrDie by way of
// LoadExternalPackages, which calls os.Exit(1). That cannot be observed from
// inside the same process: there is no panic to recover and no error to assert
// on, the process is simply gone. So the check runs in a re-executed copy of
// this test binary with the kubeconfig removed from its environment, and the
// parent asserts on the child's exit code.
//
// Without the guard the child exits 1 having printed nothing at all, which is
// precisely the failure mode this protects against.
func TestDefaultCompilerWithoutKubeConfig(t *testing.T) {
	if os.Getenv(childEnvVar) == "1" {
		// Child: constructing the compiler must not terminate the process.
		_ = DefaultCompiler.Get()
		t.Log("built DefaultCompiler with no kubeconfig")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestDefaultCompilerWithoutKubeConfig$", "-test.v")
	cmd.Env = append(os.Environ(),
		childEnvVar+"=1",
		// Remove every route the resolver has to a config: the explicit env
		// var, the home directory default, and the in-cluster service account.
		"KUBECONFIG="+filepath.Join(t.TempDir(), "does-not-exist"),
		"HOME="+t.TempDir(),
		"KUBERNETES_SERVICE_HOST=",
		"KUBERNETES_SERVICE_PORT=",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("building the compiler without a kubeconfig terminated the process: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(string(out), "built DefaultCompiler with no kubeconfig") {
		t.Fatalf("child did not reach the end of the test\noutput:\n%s", out)
	}
}

// TestDefaultCompilerInProcess repeats the check in this process so the guard
// is recorded as covered; the subprocess test above cannot contribute coverage,
// since the toolchain does not collect it from a child.
//
// It deliberately runs after that test. If the guard ever regresses this call
// takes the whole test binary down with it, and the ordering means the clearer
// failure from the subprocess test is already in the output when it happens.
func TestDefaultCompilerInProcess(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "does-not-exist"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")

	if c := DefaultCompiler.Get(); c == nil {
		t.Fatal("expected a usable compiler when no kubeconfig is available")
	}
}
