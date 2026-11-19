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

package cuetest

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// A cluster-scoped object is read back without a namespace, as the API
// server reports it, whichever namespace the Application is in.
func TestStatusClusterScoped(t *testing.T) {
	s := Subject{
		Kind: KindComponent,
		Name: "reader-role",
		Template: `
output: {
	apiVersion: "rbac.authorization.k8s.io/v1"
	kind:       "ClusterRole"
	rules: [{apiGroups: [""], resources: ["pods"], verbs: ["get"]}]
}
outputs: binding: {
	apiVersion: "rbac.authorization.k8s.io/v1"
	kind:       "ClusterRoleBinding"
	metadata: name: "\(context.name)-binding"
	roleRef: {apiGroup: "rbac.authorization.k8s.io", kind: "ClusterRole", name: context.name}
	subjects: [{kind: "Group", name: "readers", apiGroup: "rbac.authorization.k8s.io"}]
}
parameter: {}
`,
		HealthPolicy: `isHealth: context.outputs.binding.metadata.name == "reader-binding"`,
		CustomStatus: `
_ns: *context.output.metadata.namespace | "none"
message: "\(context.output.metadata.name) in \(_ns)"
`,
	}
	res, err := Status(s, Input{Context: Context{Name: "reader", Namespace: "prod"}}, Observed{})
	require.NoError(t, err)
	require.True(t, res.Healthy, "an output is found by name")
	require.Equal(t, "reader in none", res.Message, "an output is found by the component name")
}

// clusterScopedKinds holds every cluster-scoped kind the API server stores
// from Kubernetes' own groups; the review kinds are requests, never read.
func TestClusterScopedKindsMatchTheAPIServer(t *testing.T) {
	cl := testCluster(t)
	dc, err := discovery.NewDiscoveryClientForConfig(cl.Config)
	require.NoError(t, err)
	lists, err := dc.ServerPreferredResources()
	require.NoError(t, err)
	var missing []string
	for _, list := range lists {
		gv, err := k8sschema.ParseGroupVersion(list.GroupVersion)
		require.NoError(t, err)
		if gv.Group != "" && !strings.HasSuffix(gv.Group, "k8s.io") {
			continue
		}
		for _, r := range list.APIResources {
			if r.Namespaced || strings.Contains(r.Name, "/") || !slices.Contains(r.Verbs, "get") {
				continue
			}
			if gk := (k8sschema.GroupKind{Group: gv.Group, Kind: r.Kind}); !clusterScopedKinds[gk] {
				missing = append(missing, gk.String())
			}
		}
	}
	require.Empty(t, missing)
}
