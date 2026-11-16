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

package template

import (
	"context"
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/workflow/providers"
)

// velaCLIJob renders the vela-cli step for an Application in namespace with parameters, provider
// calls left unresolved, and returns the namespace and service account of the Job it applies.
func velaCLIJob(t *testing.T, namespace, parameters string) (string, string) {
	t.Helper()
	src := readWorkflowStepCue(t, "vela-cli.cue") + fmt.Sprintf(`
context: {name: "app", namespace: %q, stepName: "run", stepSessionID: "id"}
template: parameter: %s
`, namespace, parameters)
	v, err := providers.DefaultCompiler.Get().CompileStringWithOptions(context.Background(), src, cuex.DisableResolveProviderFunctions{})
	require.NoError(t, err)
	job := v.LookupPath(cue.ParsePath("template.job.$params.value"))
	ns, err := job.LookupPath(cue.ParsePath("metadata.namespace")).String()
	require.NoError(t, err)
	sa, err := job.LookupPath(cue.ParsePath("spec.template.spec.serviceAccountName")).String()
	require.NoError(t, err)
	return ns, sa
}

// The vela-cli step's Job runs where the Application is, as the service account it names there,
// default unless set: naming vela-core's own account does not move it to vela-system.
func TestTheVelaCLIJobRunsWhereTheApplicationIs(t *testing.T) {
	ns, sa := velaCLIJob(t, "team-a", `{command: ["vela", "ls"]}`)
	require.Equal(t, "team-a", ns)
	require.Equal(t, "default", sa)

	ns, sa = velaCLIJob(t, "team-a", `{command: ["vela", "ls"], serviceAccountName: "kubevela-vela-core"}`)
	require.Equal(t, "team-a", ns, "naming the controller's account does not move the Job")
	require.Equal(t, "kubevela-vela-core", sa)

	ns, _ = velaCLIJob(t, "vela-system", `{command: ["vela", "ls"], serviceAccountName: "kubevela-vela-core"}`)
	require.Equal(t, "vela-system", ns, "a platform Application in vela-system still runs it there")
}
