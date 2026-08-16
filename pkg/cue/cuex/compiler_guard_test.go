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

package cuex_test

import (
	"path/filepath"
	"testing"

	velacuex "github.com/oam-dev/kubevela/pkg/cue/cuex"
	"github.com/oam-dev/kubevela/pkg/utils/kubeconfig"
)

// TestWorkloadCompilerWithoutKubeConfig pins the guard that keeps compiler
// construction from exiting the process when nothing is resolvable.
//
// LoadExternalPackages reaches config.GetConfigOrDie by way of the shared
// client singletons, and that calls os.Exit(1) rather than returning an error.
// If this regresses, the symptom is the test binary vanishing here with no
// failure reported, the same thing that happens to `vela def render`.
func TestWorkloadCompilerWithoutKubeConfig(t *testing.T) {
	// TestMain vouches for the envtest config it supplied. Withdraw that, and
	// hide the environment, so the guard takes its skip path. Reinstated
	// afterwards so the rest of the suite still reaches its control plane.
	t.Cleanup(kubeconfig.AssumeUnavailable())

	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "does-not-exist"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")

	velacuex.WorkloadCompiler.Reload()
	if c := velacuex.WorkloadCompiler.Get(); c == nil {
		t.Fatal("expected a usable compiler when no kubeconfig is available")
	}
}
