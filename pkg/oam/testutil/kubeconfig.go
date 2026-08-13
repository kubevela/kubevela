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

	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// kubeConfigHelp is printed when no usable kubeconfig is found. It tells the
// developer exactly how to unblock the test run instead of leaving them with a
// silent process exit.
const kubeConfigHelp = `
==============================================================================
This test package requires a kubeconfig to be present.

Several kubevela packages resolve a Kubernetes client through
github.com/kubevela/pkg/util/singleton, whose config loader calls
GetConfigOrDie() and terminates the whole test binary with os.Exit(1) when no
kubeconfig can be found. Without this guard, the package would fail with NO
output at all.

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

// RequireKubeConfig verifies that a kubeconfig is loadable BEFORE any test code
// touches the kubevela/pkg singleton clients. It must be called from TestMain
// prior to m.Run().
//
// It deliberately fails fast with an actionable message rather than skipping:
// skipping would silently drop coverage in CI if the CI kubeconfig setup ever
// broke, whereas an explicit failure is always visible.
func RequireKubeConfig() {
	if _, err := config.GetConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: no usable kubeconfig: %v\n%s", err, kubeConfigHelp)
		os.Exit(1)
	}
}
