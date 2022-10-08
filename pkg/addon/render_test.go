/*
Copyright 2022 The KubeVela Authors.

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
	"strings"
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
	resourceComponent1 := `
myref: {
	type: "ref-objects"
	properties: {
		urls: ["https://hello.yaml"]
	}
}
`
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
	       },myref]
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
		Parameters:     paraDefined,
		CUETemplates:   []ElementFile{{Data: resourceComponent1}},
		AppCueTemplate: ElementFile{Data: appTemplate},
	}

	render := addonCueTemplateRender{
		addon: addon,
		inputArgs: map[string]interface{}{
			"namespace": "vela-system",
		},
	}
	app, _, err := render.renderApp()
	assert.Equal(t, err.Error(), `load app template with CUE files: output.spec.components: reference "myref" not found`)
	assert.Nil(t, app)

	addon.CUETemplates = []ElementFile{{Data: "package main\n" + resourceComponent1}}
	app, _, err = render.renderApp()
	assert.NoError(t, err)
	assert.Equal(t, len(app.Spec.Components), 2)
	str, err := json.Marshal(app.Spec.Components[0].Properties)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(str), `{"name":"vela-system"}`))
	str2, err := json.Marshal(app.Spec.Components[1].Properties)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(str2), `{"urls":["https://hello.yaml"]}`))

	assert.Equal(t, len(app.Spec.Policies), 2)
	str, err = json.Marshal(app.Spec.Policies)
	assert.NoError(t, err)
	assert.Contains(t, string(str), `"clusterLabelSelector":{}`)

	addon.Parameters = "package newp\n" + paraDefined
	addon.CUETemplates = []ElementFile{{Data: "package newp\n" + resourceComponent1}}
	addon.AppCueTemplate = ElementFile{Data: "package newp\n" + appTemplate}
	app, _, err = render.renderApp()
	assert.NoError(t, err)
	assert.Equal(t, len(app.Spec.Components), 2)

	addon.CUETemplates = []ElementFile{{Data: "package main\n" + resourceComponent1}}
	addon.Parameters = paraDefined
	addon.AppCueTemplate = ElementFile{Data: appTemplate}
	app, _, err = render.renderApp()
	assert.NoError(t, err)
	assert.Equal(t, len(app.Spec.Components), 2)

	addon.CUETemplates = []ElementFile{{Data: "package hello\n" + resourceComponent1}}
	addon.AppCueTemplate = ElementFile{Data: "package main\n" + appTemplate}
	_, _, err = render.renderApp()
	assert.Equal(t, err.Error(), `load app template with CUE files: output.spec.components: reference "myref" not found`)

	addon.CUETemplates = []ElementFile{{Data: "package hello\n" + resourceComponent1}}
	addon.Parameters = paraDefined
	addon.AppCueTemplate = ElementFile{Data: appTemplate}
	_, _, err = render.renderApp()
	assert.Equal(t, err.Error(), `load app template with CUE files: output.spec.components: reference "myref" not found`)

}

func TestOutputsRender(t *testing.T) {
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
	},
	outputs: configmap: {
       apiVersion: "v1"
       kind: "Configmap"
       metadata: {
            name: "test-cm"
            namespace: "default"
       }
       data: parameter.data
    }
`
	paraDefined := `parameter: {
	// +usage=The clusters to install
	data: "myData"
}`
	appTemplateNoOutputs := `output: {
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
	},
`

	addon := &InstallPackage{
		Meta: Meta{
			Name: "velaux",
			DeployTo: &DeployTo{
				RuntimeCluster: true,
			},
		},
		Parameters:     paraDefined,
		AppCueTemplate: ElementFile{Data: appTemplate},
	}
	render := addonCueTemplateRender{
		addon: addon,
		inputArgs: map[string]interface{}{
			"namespace": "vela-system",
		},
	}
	app, auxdata, err := render.renderApp()
	assert.NoError(t, err)
	assert.Equal(t, len(app.Spec.Components), 1)
	str, err := json.Marshal(app.Spec.Components[0].Properties)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(str), `{"name":"vela-system"}`))
	assert.Equal(t, len(auxdata), 1)
	auxStr, err := json.Marshal(auxdata[0])
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(auxStr), "myData"))
	assert.True(t, strings.Contains(string(auxStr), "addons.oam.dev/auxiliary-name"))
	assert.True(t, strings.Contains(string(auxStr), "configmap"))

	// test no error when no outputs
	addon.AppCueTemplate = ElementFile{Data: appTemplateNoOutputs}
	_, _, err = render.renderApp()
	assert.NoError(t, err)
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
	err := render.toObject(compTemplate, renderOutputCuePath, &comp)
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

func TestGenerateAppFrameworkWithCue(t *testing.T) {
	paraDefined := `parameter: {
	// +usage=The clusters to install
	clusters?: [...string]
	namespace: string
}`
	cueTemplate := `output: {
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
	cueAddon := &InstallPackage{
		Meta:           Meta{Name: "velaux", DeployTo: &DeployTo{RuntimeCluster: true}},
		AppCueTemplate: ElementFile{Data: cueTemplate},
		Parameters:     paraDefined,
	}
	app, _, err := generateAppFramework(cueAddon, map[string]interface{}{
		"namespace": "vela-system",
	})
	assert.NoError(t, err)
	assert.Equal(t, len(app.Spec.Components), 1)
	str, err := json.Marshal(app.Spec.Components[0].Properties)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(str), `{"name":"vela-system"}`))
	assert.Equal(t, len(app.Spec.Policies), 2)
	str, err = json.Marshal(app.Spec.Policies)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(str), `"clusterLabelSelector":{}`))
	assert.Equal(t, len(app.Labels), 2)
}

func TestGenerateAppFrameworkWithYamlTemplate(t *testing.T) {
	yamlAddon := &InstallPackage{
		Meta:        Meta{Name: "velaux"},
		AppTemplate: nil,
	}
	app, _, err := generateAppFramework(yamlAddon, nil)
	assert.NoError(t, err)
	assert.Equal(t, app.Spec.Components != nil, true)
	assert.Equal(t, len(app.Labels), 2)

	noCompAddon := &InstallPackage{
		Meta:        Meta{Name: "velaux"},
		AppTemplate: &v1beta1.Application{},
	}
	app, _, err = generateAppFramework(noCompAddon, nil)
	assert.NoError(t, err)
	assert.Equal(t, app.Spec.Components != nil, true)
	assert.Equal(t, len(app.Labels), 2)
}

func TestRenderCueResourceError(t *testing.T) {
	cueTemplate1 := `output: {
 type: "webservice"
 name: "velaux"
}`
	cueTemplate2 := `output: {
 type: "webservice"
 name: "velaux2"
}`
	cueTemplate3 := `nooutput: {
 type: "webservice"
 name: "velaux3"
}`
	comp, err := renderResources(&InstallPackage{
		CUETemplates: []ElementFile{
			{
				Data: cueTemplate1,
				Name: "tmplaate1.cue",
			},
			{
				Data: cueTemplate2,
				Name: "tmplaate2.cue",
			},
			{
				Data: cueTemplate3,
				Name: "tmplaate3.cue",
			},
		},
	}, nil)
	assert.NoError(t, err)
	assert.Equal(t, len(comp), 2)
}

func TestCheckCueFileHasPackageHeader(t *testing.T) {
	testCueTemplateWithPkg := `
package main

kustomizeController: {
	// About this name, refer to #429 for details.
	name: "fluxcd-kustomize-controller"
	type: "webservice"
	dependsOn: ["fluxcd-ns"]
	properties: {
		imagePullPolicy: "IfNotPresent"
		image:           _base + "fluxcd/kustomize-controller:v0.26.0"
		env: [
			{
				name:  "RUNTIME_NAMESPACE"
				value: _targetNamespace
			},
		]
		livenessProbe: {
			httpGet: {
				path: "/healthz"
				port: 9440
			}
			timeoutSeconds: 5
		}
		readinessProbe: {
			httpGet: {
				path: "/readyz"
				port: 9440
			}
			timeoutSeconds: 5
		}
		volumeMounts: {
			emptyDir: [
				{
					name:      "temp"
					mountPath: "/tmp"
				},
			]
		}
	}
	traits: [
		{
			type: "service-account"
			properties: {
				name:       "sa-kustomize-controller"
				create:     true
				privileges: _rules
			}
		},
		{
			type: "labels"
			properties: {
				"control-plane": "controller"
				// This label is kept to avoid breaking existing 
				// KubeVela e2e tests (makefile e2e-setup).
				"app": "kustomize-controller"
			}
		},
		{
			type: "command"
			properties: {
				args: controllerArgs
			}
		},
	]
}
`

	testCueTemplateWithoutPkg := `
output: {
   type: "helm"
	name: "nginx-ingress"
	properties: {
		repoType: "helm"
		url:      "https://kubernetes.github.io/ingress-nginx"
		chart:    "ingress-nginx"
		version:  "4.2.0"
		values: {
			controller: service: type: parameter["serviceType"]
		}
	}
}
`

	cueTemplate := ElementFile{Name: "test-file.cue", Data: testCueTemplateWithPkg}
	ok, err := checkCueFileHasPackageHeader(cueTemplate)
	assert.NoError(t, err)
	assert.Equal(t, true, ok)

	cueTemplate = ElementFile{Name: "test-file-without-pkg.cue", Data: testCueTemplateWithoutPkg}
	ok, err = checkCueFileHasPackageHeader(cueTemplate)
	assert.NoError(t, err)
	assert.Equal(t, false, ok)
}
