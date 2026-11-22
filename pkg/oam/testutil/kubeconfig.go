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

package testutil

import (
	"fmt"
	"os"

	"github.com/oam-dev/kubevela/pkg/utils/kubeconfig"
)

// kubeConfigHelp is printed when no usable kubeconfig is found. It tells the
// developer exactly how to unblock the run instead of leaving them with a
// silent process exit.
const kubeConfigHelp = `
==============================================================================
This test package requires a kubeconfig to be present.

It resolves a Kubernetes client through github.com/kubevela/pkg/util/singleton,
whose loader calls GetConfigOrDie() and terminates the whole test binary with
os.Exit(1) when no kubeconfig can be found. Without this guard the package
fails with NO output at all, which reads as a broken test suite.

The tests never contact the cluster, so any parseable kubeconfig works.
Either point KUBECONFIG at an existing config, or create a dummy one:

  mkdir -p ~/.kube && cat > ~/.kube/config <<'EOF'
  apiVersion: v1
  kind: Config
  clusters:
  - cluster: {server: "https://127.0.0.1:1", insecure-skip-tls-verify: true}
    name: dummy
  contexts:
  - context: {cluster: dummy, user: dummy}
    name: dummy
  current-context: dummy
  users:
  - name: dummy
    user: {token: dummy}
  EOF

See contribute/testcode-guidance.md for details.
==============================================================================
`

// RequireKubeConfig verifies that a kubeconfig is resolvable BEFORE any test
// code touches the kubevela/pkg singleton clients. Call it from TestMain,
// before m.Run.
//
// The probe is kubeconfig.Check, the same one the production call sites in
// pkg/cue/cuex and pkg/workflow/providers use to degrade instead of exiting, so
// there is one answer to "is there a config" rather than two implementations of
// the question that can disagree.
//
// It fails fast rather than skipping: a skip would silently drop the package
// from CI if the CI kubeconfig setup ever broke, and a package that quietly
// stops running is the failure this guard exists to prevent. Exiting here is
// not the silent exit being fixed -- there is no test framework to report into
// before m.Run, and unlike GetConfigOrDie it says what is wrong and how to fix
// it.
func RequireKubeConfig() {
	if err := kubeconfig.Check(); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: no usable kubeconfig: %v\n%s", err, kubeConfigHelp)
		os.Exit(1)
	}
}
