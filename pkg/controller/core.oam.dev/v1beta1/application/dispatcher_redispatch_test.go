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

package application

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/sources"
)

func componentOfDefinition(annotations map[string]string) *appfile.Component {
	return &appfile.Component{FullTemplate: &appfile.Template{
		ComponentDefinition: &v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Annotations: annotations}},
	}}
}

func TestDefinitionRequestsRedispatch(t *testing.T) {
	for name, tc := range map[string]struct {
		comp *appfile.Component
		want bool
	}{
		"definition asks for it": {
			comp: componentOfDefinition(map[string]string{types.AnnoDefinitionRedispatchOnWorkflowRun: "true"}),
			want: true,
		},
		"any other value": {
			comp: componentOfDefinition(map[string]string{types.AnnoDefinitionRedispatchOnWorkflowRun: "yes"}),
		},
		"definition without the annotation": {
			comp: componentOfDefinition(map[string]string{types.AnnoDefinitionDescription: "webservice"}),
		},
		"workload definition, no component definition": {
			comp: &appfile.Component{FullTemplate: &appfile.Template{WorkloadDefinition: &v1beta1.WorkloadDefinition{}}},
		},
		"no template": {
			comp: &appfile.Component{},
		},
		"no component": {},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, definitionRequestsRedispatch(tc.comp))
		})
	}
}

// The built-in addon and module definitions render from a registry, so a
// workflow run must re-apply them even when their properties are unchanged: a
// new tag or a re-pushed tag only shows up in what they render.
func TestAddonAndModuleDefinitionsRequestRedispatch(t *testing.T) {
	for _, name := range []string{"addon", "module"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("../../../../../charts/vela-core/definitions/" + name + ".yaml")
			require.NoError(t, err)
			var def struct {
				Metadata metav1.ObjectMeta `json:"metadata"`
			}
			// The chart file has Helm template lines in it, but metadata is plain YAML.
			require.NoError(t, yaml.Unmarshal(metadataBlock(t, raw), &def))
			require.Equal(t, "true", def.Metadata.Annotations[types.AnnoDefinitionRedispatchOnWorkflowRun])
		})
	}
}

// metadataBlock returns the top-level "metadata:" section of a chart
// definition file, without the lines that hold Helm templates.
func metadataBlock(t *testing.T, raw []byte) []byte {
	t.Helper()
	var out []string
	inMetadata := false
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "metadata:" {
			inMetadata = true
		} else if inMetadata && line != "" && !strings.HasPrefix(line, " ") {
			break
		}
		if inMetadata && !strings.Contains(line, "{{") {
			out = append(out, line)
		}
	}
	require.NotEmpty(t, out, "no metadata block")
	return []byte(strings.Join(out, "\n"))
}

// A component whose render was delivered component reads is applied on every
// workflow run: what it renders follows the producers' live objects, which can
// change while its own properties do not.
func TestComponentReadsRequestRedispatch(t *testing.T) {
	withScope := func(scope map[string]interface{}) *appfile.Component {
		ctx := context.Background()
		if scope != nil {
			ctx = sources.WithComponentScope(ctx, scope)
		}
		return &appfile.Component{Ctx: velaprocess.NewContext(velaprocess.ContextData{Ctx: ctx, AppName: "app", CompName: "api"})}
	}
	require.True(t, componentReadsRequestRedispatch(withScope(map[string]interface{}{"db": map[string]interface{}{}})))
	require.False(t, componentReadsRequestRedispatch(withScope(nil)), "no reads delivered")
	require.False(t, componentReadsRequestRedispatch(&appfile.Component{}), "no render context")
	require.False(t, componentReadsRequestRedispatch(nil))
}

// SkipApplyWorkload means a trait applies the workload, not that nothing is
// applied: the traits are still dispatched, and the component's reads are
// delivered into them. So a read asks for the run, while auto-update and a
// definition's own request stay tied to the workload that is not being applied.
func TestRedispatchOnRunForAManagedWorkload(t *testing.T) {
	reader := func(skip bool) *appfile.Component {
		ctx := sources.WithComponentScope(context.Background(), map[string]interface{}{"db": map[string]interface{}{}})
		return &appfile.Component{
			SkipApplyWorkload: skip,
			Ctx:               velaprocess.NewContext(velaprocess.ContextData{Ctx: ctx, AppName: "app", CompName: "api"}),
		}
	}
	plain := func(skip bool) *appfile.Component {
		return &appfile.Component{
			SkipApplyWorkload: skip,
			Ctx:               velaprocess.NewContext(velaprocess.ContextData{Ctx: context.Background(), AppName: "app", CompName: "api"}),
		}
	}

	require.True(t, redispatchOnRun(reader(true), false), "a managed workload's reads still ask for the run")
	require.True(t, redispatchOnRun(reader(false), false))
	require.False(t, redispatchOnRun(plain(true), true), "auto-update does not reach a workload a trait manages")
	require.True(t, redispatchOnRun(plain(false), true), "and does reach one being applied")
	require.False(t, redispatchOnRun(nil, true))
}
