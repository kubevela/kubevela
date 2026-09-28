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

package appfile

import (
	"encoding/json"
	"testing"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/sources"
)

func rawProps(t *testing.T, v map[string]interface{}) *runtime.RawExtension {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: b}
}

func comp(t *testing.T, name string, props map[string]interface{}, traits ...map[string]interface{}) common.ApplicationComponent {
	c := common.ApplicationComponent{Name: name, Type: "webservice"}
	if props != nil {
		c.Properties = rawProps(t, props)
	}
	for _, tr := range traits {
		c.Traits = append(c.Traits, common.ApplicationTrait{Type: "labels", Properties: rawProps(t, tr)})
	}
	return c
}

func TestComponentReadsFindsEveryRead(t *testing.T) {
	c := comp(t, "api",
		map[string]interface{}{
			"env": []interface{}{
				map[string]interface{}{"name": "DB", "value": `pg://$(component.db.output.status.endpoint):5432`},
			},
			"image": `$(source.img.image)`,
		},
		map[string]interface{}{"svc": `$(component.cache.outputs.svc.spec.clusterIP)`},
	)
	reads, err := sources.ComponentReads(c)
	require.NoError(t, err)
	require.Equal(t, []sources.ComponentRead{
		{Trait: -1, Producer: "db", Path: []string{"output", "status", "endpoint"}},
		{Trait: 0, Producer: "cache", Path: []string{"outputs", "svc", "spec", "clusterIP"}},
	}, reads)
}

func TestValidateComponentReads(t *testing.T) {
	db := comp(t, "db", nil)
	read := func(expr string) common.ApplicationComponent {
		return comp(t, "api", map[string]interface{}{"x": expr})
	}

	for _, tc := range []struct {
		name  string
		comps []common.ApplicationComponent
		err   string
	}{
		{"workload output", []common.ApplicationComponent{db, read(`$(component.db.output.status.endpoint)`)}, ""},
		{"trait resource", []common.ApplicationComponent{db, read(`$(component.db.outputs.svc.spec.clusterIP)`)}, ""},
		{"unknown component", []common.ApplicationComponent{db, read(`$(component.nope.output.status.x)`)},
			`component "api" reads component.nope.output.status.x, but the application has no component "nope"`},
		{"itself", []common.ApplicationComponent{db, comp(t, "api", map[string]interface{}{"x": `$(component.api.output.status.x)`})},
			`component "api" reads its own output`},
		{"whole component", []common.ApplicationComponent{db, read(`$(component.db)`)},
			`read component.db.output or component.db.outputs.<resource>`},
		{"unnamed trait resource", []common.ApplicationComponent{db, read(`$(component.db.outputs)`)},
			`read component.db.output or component.db.outputs.<resource>`},
		{"not output", []common.ApplicationComponent{db, read(`$(component.db.properties.image)`)},
			`read component.db.output or component.db.outputs.<resource>`},
		// The whole output is delivered, so a guard sees what is really there.
		{"guarded", []common.ApplicationComponent{db, read(`$(has(component.db.output.status.x) ? component.db.output.status.x : "none")`)}, ""},
		{"at a cluster", []common.ApplicationComponent{db, read(`$(component.db.cluster("data").output.status.x)`)}, ""},
		{"at a cluster and namespace", []common.ApplicationComponent{db, read(`$(component.db.cluster("data").namespace("orders").output.status.x)`)}, ""},
		{"every placement is not a read", []common.ApplicationComponent{db, read(`$(component.db.placements("hub")[0].output.status.x)`)},
			`undeclared reference to 'placements'`},
		{"a namespace beside me", []common.ApplicationComponent{db, read(`$(component.db.namespace("team-a").output.status.x)`)}, ""},
		{"namespace before cluster", []common.ApplicationComponent{db, read(`$(component.db.namespace("a").cluster("b").output.status.x)`)},
			`name a placement with .cluster("<cluster>")`},
		{"a qualifier elsewhere", []common.ApplicationComponent{db, read(`$(component.db.output.cluster("data").status.x)`)},
			`cluster and namespace go straight after component.<name>`},
		{"a computed placement", []common.ApplicationComponent{db, read(`$(component.db.cluster(context.cluster).output.status.x)`)},
			`cluster() takes a literal string`},
		{"cycle through reads", []common.ApplicationComponent{
			comp(t, "db", map[string]interface{}{"x": `$(component.api.output.status.x)`}),
			read(`$(component.db.output.status.x)`)},
			`cycle: db -> api -> db`},
		{"cycle through dependsOn", []common.ApplicationComponent{
			{Name: "db", Type: "webservice", DependsOn: []string{"api"}},
			read(`$(component.db.output.status.x)`)},
			`cycle: db -> api -> db`},
		{"cycle through explicit inputs", []common.ApplicationComponent{
			{Name: "db", Type: "webservice", Inputs: wfTypesv1alpha1.StepInputs{{From: "api-host", ParameterKey: "host"}}},
			func() common.ApplicationComponent {
				c := read(`$(component.db.output.status.x)`)
				c.Outputs = wfTypesv1alpha1.StepOutputs{{Name: "api-host", ValueFrom: "output.status.host"}}
				return c
			}()},
			`cycle: db -> api -> db`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateComponentReads(v1beta1.ApplicationSpec{Components: tc.comps, Policies: []v1beta1.AppPolicy{
				{Name: "hub", Type: "topology"}, {Name: "edge", Type: "topology"}, {Name: "only-db", Type: "override"},
			}})
			if tc.err == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.err)
		})
	}
}

// Steps run in order unless the workflow says otherwise, so a reader deployed by
// an earlier step than its producer waits for a step that never starts.
