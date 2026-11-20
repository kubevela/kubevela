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
	"os"
	"testing"

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMain(m *testing.M) {
	cuex.EnableExternalPackageForDefaultCompiler = false
	// Nothing here may reach a real cluster, whatever the kubeconfig says.
	singleton.KubeClient.Set(fake.NewClientBuilder().Build())
	code := m.Run()
	_ = StopExecCluster()
	os.Exit(code)
}

const componentTemplate = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {
		name:      context.name
		namespace: context.namespace
		labels: app: context.appName
	}
	spec: {
		replicas: parameter.replicas
		template: spec: containers: [{image: parameter.image}]
	}
}
outputs: service: {
	apiVersion: "v1"
	kind:       "Service"
	metadata: name: "\(context.name)-svc"
}
parameter: {
	image:     string
	replicas: *1 | int
}
`

const traitTemplate = `
patch: spec: replicas: parameter.replicas
outputs: hpa: {
	apiVersion: "autoscaling/v2"
	kind:       "HorizontalPodAutoscaler"
	metadata: name: context.name
}
parameter: replicas: int
`

func TestRenderComponent(t *testing.T) {
	r := require.New(t)
	got, err := Render(Subject{Kind: KindComponent, Name: "web", Template: componentTemplate}, Input{
		Context:   Context{Name: "api"},
		Parameter: map[string]any{"image": "nginx"},
	})
	r.NoError(err)
	r.Equal("api", got.Output["metadata"].(map[string]any)["name"])
	r.Equal("default", got.Output["metadata"].(map[string]any)["namespace"], "unset context fields take defaults")
	labels := got.Output["metadata"].(map[string]any)["labels"].(map[string]any)
	r.Equal("test-app", labels["app"], "the template's own labels")
	r.Equal("api", labels["app.oam.dev/component"], "and the labels the controller applies")
	r.Equal("web", labels["workload.oam.dev/type"])
	r.EqualValues(1, got.Output["spec"].(map[string]any)["replicas"], "parameter defaults apply")
	r.Equal("api-svc", got.Outputs["service"]["metadata"].(map[string]any)["name"])
}

func TestRenderComponentError(t *testing.T) {
	_, err := Render(Subject{Kind: KindComponent, Name: "web", Template: componentTemplate}, Input{
		Parameter: map[string]any{"image": 3},
	})
	require.ErrorContains(t, err, "image")
}

func TestRenderTrait(t *testing.T) {
	r := require.New(t)
	got, err := Render(Subject{Kind: KindTrait, Name: "scaler", Template: traitTemplate}, Input{
		Parameter: map[string]any{"replicas": 4},
		Workload: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"spec":       map[string]any{"paused": true},
		},
	})
	r.NoError(err)
	r.EqualValues(4, got.Output["spec"].(map[string]any)["replicas"], "the trait patches the workload")
	r.Equal(true, got.Output["spec"].(map[string]any)["paused"], "unpatched fields survive")
	r.Equal("HorizontalPodAutoscaler", got.Traits["scaler"].Outputs["hpa"]["kind"], "a trait's outputs are filed under its type")
}

func TestRenderTraitConflict(t *testing.T) {
	_, err := Render(Subject{Kind: KindTrait, Name: "scaler", Template: traitTemplate}, Input{
		Parameter: map[string]any{"replicas": 4},
		Workload:  map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "spec": map[string]any{"replicas": 1}},
	})
	require.ErrorContains(t, err, "conflicting values 4 and 1", "a patch over a concrete value fails as it does in the controller")
}

func TestRenderTraitNeedsWorkload(t *testing.T) {
	_, err := Render(Subject{Kind: KindTrait, Name: "scaler", Template: traitTemplate}, Input{
		Parameter: map[string]any{"replicas": 4},
	})
	require.ErrorContains(t, err, "workload")
}
