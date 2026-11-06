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

package addon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestLoadSchedulingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scheduling.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
nodeSelector:
  disktype: ssd
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
        - matchExpressions:
            - key: node-type
              operator: In
              values: ["infra"]
tolerations:
  - key: dedicated
    operator: Exists
    effect: NoSchedule
`), 0o600))

	props, err := LoadSchedulingFile(path)
	require.NoError(t, err)
	require.Equal(t, "ssd", props.NodeSelector["disktype"])
	require.NotEmpty(t, props.Affinity)
	require.Len(t, props.Tolerations, 1)

	aff := props.ToAffinityTraitProperties()
	require.Equal(t, "ssd", aff["nodeSelector"].(map[string]string)["disktype"])
	require.Contains(t, aff, "affinity")
	require.Contains(t, aff, "tolerations")
}

func TestExtractAndInjectSchedulingArgs(t *testing.T) {
	props := SchedulingProperties{
		NodeSelector: map[string]string{"zone": "a"},
	}
	args := map[string]interface{}{"replicas": 2}
	InjectSchedulingIntoArgs(args, props)
	require.Contains(t, args, SchedulingArg)

	got, err := ExtractSchedulingFromArgs(args)
	require.NoError(t, err)
	require.Equal(t, "a", got.NodeSelector["zone"])
	require.NotContains(t, args, SchedulingArg)
	require.Equal(t, 2, args["replicas"])
}

func TestAttachSchedulingTrait(t *testing.T) {
	app := &v1beta1.Application{}
	app.Spec.Components = []common2.ApplicationComponent{
		{Name: "web", Type: "webservice", Traits: []common2.ApplicationTrait{
			{Type: DeprecatedSchedulingTraitType},
		}},
		{Name: "addon-demo-ns", Type: "k8s-objects"},
		{Name: "chart", Type: "helmchart"},
		{Name: "provider-kit", Type: "module"},
	}
	props := SchedulingProperties{NodeSelector: map[string]string{"disktype": "ssd"}}
	require.NoError(t, AttachSchedulingTrait(app, props))

	require.Len(t, app.Spec.Components[0].Traits, 1)
	require.Equal(t, AffinityTraitType, app.Spec.Components[0].Traits[0].Type)
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(app.Spec.Components[0].Traits[0].Properties.Raw, &got))
	require.Equal(t, "ssd", got["nodeSelector"].(map[string]interface{})["disktype"])

	require.Empty(t, app.Spec.Components[1].Traits)
	require.Equal(t, AffinityTraitType, app.Spec.Components[2].Traits[0].Type)
	require.Equal(t, AffinityTraitType, app.Spec.Components[3].Traits[0].Type, "type: module gets affinity trait")
}

func TestAttachSchedulingTraitTopology(t *testing.T) {
	app := &v1beta1.Application{}
	app.Spec.Components = []common2.ApplicationComponent{
		{Name: "web", Type: "webservice"},
	}
	props := SchedulingProperties{
		TopologySpreadConstraints: []interface{}{
			map[string]interface{}{
				"maxSkew":           1,
				"topologyKey":       "kubernetes.io/hostname",
				"whenUnsatisfiable": "DoNotSchedule",
				"labelSelector":     map[string]interface{}{"matchLabels": map[string]interface{}{"app": "web"}},
			},
		},
	}
	require.NoError(t, AttachSchedulingTrait(app, props))
	require.Len(t, app.Spec.Components[0].Traits, 1)
	require.Equal(t, AffinityTraitType, app.Spec.Components[0].Traits[0].Type)
	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(app.Spec.Components[0].Traits[0].Properties.Raw, &got))
	raw, ok := got["topologySpreadConstraints"].([]interface{})
	require.True(t, ok)
	require.Len(t, raw, 1)
	c := raw[0].(map[string]interface{})
	require.EqualValues(t, 1, c["maxSkew"])
	require.Equal(t, "kubernetes.io/hostname", c["topologyKey"])
	require.Equal(t, "DoNotSchedule", c["whenUnsatisfiable"])
}

func TestSchedulingPropertiesEmpty(t *testing.T) {
	require.True(t, SchedulingProperties{}.Empty())
	require.False(t, SchedulingProperties{SchedulerName: "default-scheduler"}.Empty())
}

func TestMergeNodeSelector(t *testing.T) {
	props := SchedulingProperties{NodeSelector: map[string]string{"disktype": "hdd"}}
	props.MergeNodeSelector(map[string]string{"disktype": "ssd", "node-type": "infra"})
	require.Equal(t, "ssd", props.NodeSelector["disktype"])
	require.Equal(t, "infra", props.NodeSelector["node-type"])

	empty := SchedulingProperties{}
	empty.MergeNodeSelector(map[string]string{"zone": "a"})
	require.Equal(t, "a", empty.NodeSelector["zone"])
}

// GitHub #5362: AttachSchedulingTrait puts affinity onto webservice/helmchart/module
// components and skips *-ns helpers / raw. CLI flag wiring is covered by
// TestInjectSchedulingArgs in references/cli.
func TestAttachSchedulingTraitIssue5362Matrix(t *testing.T) {
	app := &v1beta1.Application{}
	app.Spec.Components = []common2.ApplicationComponent{
		{Name: "addon-demo-ns", Type: "k8s-objects"},
		{Name: "velaux", Type: "webservice"},
		{Name: "chart", Type: "helmchart"},
		{Name: "provider-kit", Type: "module"},
		{Name: "raw-blob", Type: "raw"},
		{Name: "aux-objects", Type: "k8s-objects"},
		{Name: "backend-ns", Type: "webservice"},
	}
	props := SchedulingProperties{
		NodeSelector: map[string]string{"disktype": "ssd", "node-type": "infra"},
		Affinity: map[string]interface{}{
			"nodeAffinity": map[string]interface{}{
				"requiredDuringSchedulingIgnoredDuringExecution": map[string]interface{}{
					"nodeSelectorTerms": []interface{}{
						map[string]interface{}{
							"matchExpressions": []interface{}{
								map[string]interface{}{"key": "node-type", "operator": "In", "values": []interface{}{"infra"}},
							},
						},
					},
				},
			},
		},
		Tolerations: []interface{}{
			map[string]interface{}{"key": "dedicated", "operator": "Exists", "effect": "NoSchedule"},
		},
		SchedulerName: "default-scheduler",
	}
	require.NoError(t, AttachSchedulingTrait(app, props))

	require.Empty(t, app.Spec.Components[0].Traits, "k8s-objects *-ns helper skipped")
	require.Empty(t, app.Spec.Components[4].Traits, "raw skipped")

	for _, idx := range []int{1, 2, 3, 5, 6} {
		comp := app.Spec.Components[idx]
		require.Len(t, comp.Traits, 1, comp.Name)
		require.Equal(t, AffinityTraitType, comp.Traits[0].Type)
		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(comp.Traits[0].Properties.Raw, &got))
		ns := got["nodeSelector"].(map[string]interface{})
		require.Equal(t, "ssd", ns["disktype"])
		require.Equal(t, "infra", ns["node-type"])
		require.Contains(t, got, "affinity")
		require.Contains(t, got, "tolerations")
		require.Equal(t, "default-scheduler", got["schedulerName"])
	}
}

func TestExtractSchedulingFromArgsMalformedKeepsKey(t *testing.T) {
	args := map[string]interface{}{
		SchedulingArg: "not-an-object",
		"replicas":    2,
	}
	_, err := ExtractSchedulingFromArgs(args)
	require.Error(t, err)
	require.Contains(t, args, SchedulingArg, "malformed key must remain for the caller to see")
	require.Equal(t, 2, args["replicas"])
}
