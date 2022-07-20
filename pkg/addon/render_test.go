/*
Copyright 2021 The KubeVela Authors.

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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestRenderAppTemplate(t *testing.T) {
	paraDefined := `parameter: {
	// +usage=The clusters to install
	clusters?: [...string]
	namespace: string
}`
	appTemplate := `output: {
	   apiVersion: "core.oam.dev/v1beta1"
	   kind: "Application"
	   metadata: {
	       name:  "velaux"
	       namespace: "vela-system"
	   }
	   spec: {
	       components: [{
	           type: "k8s-objects"
	           name: "vela-namespace"
	           properties: objects: [{
	               apiVersion: "v1"
	               kind: "Namespace"
	               metadata: name: parameter.namespace
	           }]
	       }]
	       policies: [{
	           type: "shared-resource"
	           name: "namespace"
	           properties: rules: [{selector: resourceTypes: ["Namespace"]}]
	       }, {
	           type: "topology"
	           name: "deploy-topology"
	           properties: {
	               if parameter.clusters != _|_ {
	                   clusters: parameter.clusters
	               }
	               if parameter.clusters == _|_ {
	                   clusterLabelSelector: {}
	               }
	               namespace: parameter.namespace
	           }
	       }]
	   }
	}`
	addon := &InstallPackage{
		Meta: Meta{
			Name: "velaux",
			DeployTo: &DeployTo{
				RuntimeCluster: true,
			},
		},
		Parameters: paraDefined,
	}

	render := addonCueTemplateRender{
		addon: addon,
		inputArgs: map[string]interface{}{
			"namespace": "vela-system",
		},
	}
	app := v1beta1.Application{}
	err := render.toObject(appTemplate, &app)
	assert.NoError(t, err)
	assert.Equal(t, len(app.Spec.Components), 1)
	str, err := json.Marshal(app.Spec.Components[0].Properties)
	assert.NoError(t, err)
	assert.Equal(t, `{"objects":[{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"vela-system"}}]}`, string(str))

	assert.Equal(t, len(app.Spec.Policies), 2)
	str, err = json.Marshal(app.Spec.Policies)
	assert.NoError(t, err)
	assert.Equal(t, `[{"name":"namespace","type":"shared-resource","properties":{"rules":[{"selector":{"resourceTypes":["Namespace"]}}]}},{"name":"deploy-topology","type":"topology","properties":{"clusterLabelSelector":{},"namespace":"vela-system"}}]`, string(str))
}

func TestAppComponentRender(t *testing.T) {
	paraDefined := `parameter: {
	image: string
}`
	compTemplate := `output: {
       type: "webservice"
       name: "velaux"
       properties: {
          image: parameter.image}
}`
	addon := &InstallPackage{
		Meta: Meta{
			Name: "velaux",
			DeployTo: &DeployTo{
				RuntimeCluster: true,
			},
		},
		Parameters: paraDefined,
	}

	render := addonCueTemplateRender{
		addon: addon,
		inputArgs: map[string]interface{}{
			"image": "1.4.1",
		},
	}
	comp := common.ApplicationComponent{}
	err := render.toObject(compTemplate, &comp)
	assert.NoError(t, err)
	assert.Equal(t, comp.Name, "velaux")
	assert.Equal(t, comp.Type, "webservice")
	str, err := json.Marshal(comp.Properties)
	assert.NoError(t, err)
	assert.Equal(t, `{"image":"1.4.1"}`, string(str))
}

func TestCheckNeedAttachTopologyPolicy(t *testing.T) {
	addon1 := &InstallPackage{
		Meta: Meta{
			DeployTo: nil,
		},
	}
	assert.Equal(t, checkNeedAttachTopologyPolicy(nil, addon1), false)

	addon2 := &InstallPackage{
		Meta: Meta{
			DeployTo: &DeployTo{RuntimeCluster: false},
		},
	}
	assert.Equal(t, checkNeedAttachTopologyPolicy(nil, addon2), false)

	addon3 := &InstallPackage{
		Meta: Meta{
			DeployTo: &DeployTo{RuntimeCluster: true},
		},
	}
	assert.Equal(t, checkNeedAttachTopologyPolicy(&v1beta1.Application{Spec: v1beta1.ApplicationSpec{Policies: []v1beta1.AppPolicy{{
		Type: v1alpha1.TopologyPolicyType,
	}}}}, addon3), false)

	addon4 := &InstallPackage{
		Meta: Meta{
			DeployTo: &DeployTo{RuntimeCluster: true},
		},
	}
	assert.Equal(t, checkNeedAttachTopologyPolicy(&v1beta1.Application{Spec: v1beta1.ApplicationSpec{Policies: []v1beta1.AppPolicy{{
		Type: v1alpha1.SharedResourcePolicyType,
	}}}}, addon4), true)
}
