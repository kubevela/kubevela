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
	"testing"

	"github.com/stretchr/testify/require"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
	"github.com/oam-dev/kubevela/pkg/features"
)

const validateParent = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: name: context.name
	spec: replicas: parameter.replicas
}

parameter: {
	image:    string
	replicas: *1 | int
}
`

const validateChild = `
$super: properties: {image: parameter.image}

parameter: {
	tenant: string
	image:  string
}
`

// Validating an extending component must not unify values from two compilers.
//
// Each cuex CompileString builds its own cue.Context, so a schema read from the
// chain and a value compiled here are from different runtimes. Grafting one onto
// the other panics with "values are not from the same runtime", which in the
// Application webhook surfaces as a denied request with a panic in the message.
func TestValidatingAnExtendingComponentDoesNotCrossRuntimes(t *testing.T) {
	withInheritance(t)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate,
		features.EnableCueValidation, true)

	wl := &Component{
		Name:   "billing-api",
		Type:   "tenant-webservice",
		Params: map[string]interface{}{"tenant": "acme", "image": "nginx:1.27"},
		FullTemplate: &Template{
			TemplateStr: validateChild,
			Ancestors:   []inherit.Level{{Name: "webservice", Template: validateParent}},
		},
		CapabilityCategory: "CUE",
	}

	app := &Appfile{
		Name:      "billing",
		Namespace: "acme",
		app:       &v1beta1.Application{},
	}

	ctxData := velaprocess.ContextData{
		CompName:  "billing-api",
		AppName:   "billing",
		Namespace: "acme",
	}

	require.NotPanics(t, func() {
		err := (&Parser{}).ValidateComponentParams(ctxData, wl, app)
		require.NoError(t, err)
	}, "a chain schema and a locally compiled value are from different runtimes")
}

// And a component that extends nothing is unaffected.
func TestValidatingAnOrdinaryComponentStillWorks(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate,
		features.EnableCueValidation, true)

	wl := &Component{
		Name:               "plain",
		Type:               "webservice",
		Params:             map[string]interface{}{"image": "nginx:1.27"},
		FullTemplate:       &Template{TemplateStr: validateParent},
		CapabilityCategory: "CUE",
	}

	app := &Appfile{Name: "app", Namespace: "acme", app: &v1beta1.Application{}}
	ctxData := velaprocess.ContextData{CompName: "plain", AppName: "app", Namespace: "acme"}

	require.NoError(t, (&Parser{}).ValidateComponentParams(ctxData, wl, app))
}
