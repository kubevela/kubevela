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

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	v1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	controllercommon "github.com/oam-dev/kubevela/pkg/controller/common"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

func runDryRunCommand(t *testing.T, flags ...string) string {
	t.Helper()
	global := &v1beta1.PolicyDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-global", Namespace: "default"},
		Spec: v1beta1.PolicyDefinitionSpec{Scope: v1beta1.ApplicationScope, Global: true,
			Schematic: &common.Schematic{CUE: &common.CUE{Template: `parameter: {}, output: labels: {"ns-global": "set"}`}}},
	}
	file := filepath.Join(t.TempDir(), "app.yaml")
	require.NoError(t, os.WriteFile(file, []byte("apiVersion: core.oam.dev/v1beta1\nkind: Application\nmetadata:\n  name: flags-app\n  namespace: default\nspec:\n  components: []\n"), 0o600))
	args := common2.Args{}
	args.SetClient(fake.NewClientBuilder().WithScheme(common2.Scheme).WithObjects(global).Build())
	var out bytes.Buffer
	cmd := NewPolicyDryRunCommand(args, cmdutil.IOStreams{Out: &out, ErrOut: &out})
	cmd.SetArgs(append([]string{"-f", file}, flags...))
	require.NoError(t, cmd.Execute())
	return out.String()
}

func TestPolicyDryRunControllerFlags(t *testing.T) {
	t.Run("the table names the flags it assumed", func(t *testing.T) {
		out := runDryRunCommand(t)
		require.Contains(t, out, "--allow-namespace-global-policies=true --allow-skip-global-policies=true")
	})

	t.Run("by default the namespace global applies", func(t *testing.T) {
		out := runDryRunCommand(t, "-o", "json")
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &got), out)
		policies, ok := got["policies"].([]any)
		require.True(t, ok, "the JSON output lists policies: %s", out)
		require.Len(t, policies, 1, "the namespace global applies")
		require.Equal(t, "ns-global", policies[0].(map[string]any)["name"])
	})

	t.Run("a flag reaches the simulation and the output", func(t *testing.T) {
		out := runDryRunCommand(t, "--allow-namespace-global-policies=false", "-o", "json")
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &got), out)
		require.Equal(t, map[string]any{"allowNamespaceGlobalPolicies": false, "allowSkipGlobalPolicies": true}, got["assumedControllerFlags"])
		policies, ok := got["policies"].([]any)
		require.True(t, ok, "the JSON output lists policies: %s", out)
		require.Empty(t, policies, "restricted, the namespace global does not apply")
		require.True(t, controllercommon.AllowNamespaceGlobalPolicies, "the command restores the process flags")
	})
}
