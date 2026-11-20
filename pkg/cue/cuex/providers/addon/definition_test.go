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

package addon_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/pkg/addon/service/api"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/deftest"
	"github.com/oam-dev/kubevela/pkg/features"
)

const addonDefinitionPath = "../../../../../vela-templates/definitions/internal/component/addon.cue"

type capturingRenderer struct{ req api.AddonRequest }

func (c *capturingRenderer) RenderAddon(_ context.Context, req api.AddonRequest) (*api.AddonResult, error) {
	c.req = req
	return &api.AddonResult{Application: map[string]interface{}{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "Application",
		"metadata":   map[string]interface{}{"name": "addon-" + req.Name},
	}}, nil
}

func TestAddonDefinition_PassesTheInstallingApplicationsNamespace(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
		features.EnableAddonComponent, true)
	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })
	fake := &capturingRenderer{}
	api.SetDefaultRenderer(fake)

	ctx := process.NewContext(process.ContextData{
		Namespace: "kit-tenant",
		AppName:   "installer",
		CompName:  "tenant-widgets",
		Ctx:       context.Background(),
	})
	err := definition.NewWorkloadAbstractEngine("tenant-widgets").
		Complete(ctx, deftest.Template(t, addonDefinitionPath), map[string]interface{}{})
	require.NoError(t, err)

	assert.Equal(t, "tenant-widgets", fake.req.Name)
	assert.Equal(t, "kit-tenant", fake.req.Namespace)
}
