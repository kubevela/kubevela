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

package definition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/kubevela/workflow/pkg/cue/packages"
	wfprocess "github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/process"
)

func TestWorkloadTemplateComplete(t *testing.T) {
	testCases := map[string]struct {
		workloadTemplate string
		params           map[string]interface{}
		expectObj        runtime.Object
		expAssObjs       map[string]runtime.Object
		category         types.CapabilityCategory
		hasCompileErr    bool
	}{
		"only contain an output": {
			workloadTemplate: `
output:{
	apiVersion: "apps/v1"
    kind: "Deployment"
	metadata: name: context.name
    spec: replicas: parameter.replicas
}
parameter: {
	replicas: *1 | int
	type: string
	host: string
}
`,
			params: map[string]interface{}{
				"replicas": 2,
				"type":     "ClusterIP",
				"host":     "example.com",
			},
			expectObj: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "test"},
				"spec":       map[string]interface{}{"replicas": int64(2)},
			}},
			hasCompileErr: false,
		},
		"contain output and outputs": {
			workloadTemplate: `
output:{
	apiVersion: "apps/v1"
    kind: "Deployment"
	metadata: name: context.name
    spec: replicas: parameter.replicas
}
outputs: service: {
	apiVersion: "v1"
    kind: "Service"
	metadata: name: context.name
    spec: type: parameter.type
}
outputs: ingress: {
	apiVersion: "extensions/v1beta1"
    kind: "Ingress"
	metadata: name: context.name
    spec: rules: [{host: parameter.host}]
}

parameter: {
	replicas: *1 | int
	type: string
	host: string
}
`,
			params: map[string]interface{}{
				"replicas": 2,
				"type":     "ClusterIP",
				"host":     "example.com",
			},
			expectObj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "test"},
					"spec": map[string]interface{}{
						"replicas": int64(2),
					},
				},
			},
			expAssObjs: map[string]runtime.Object{
				"service": &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1", "kind": "Service", "metadata": map[string]interface{}{"name": "test"},
						"spec": map[string]interface{}{"type": "ClusterIP"},
					},
				},
				"ingress": &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "extensions/v1beta1", "kind": "Ingress", "metadata": map[string]interface{}{
							"name": "test",
						}, "spec": map[string]interface{}{
							"rules": []interface{}{
								map[string]interface{}{
									"host": "example.com",
								},
							},
						},
					},
				},
			},
			hasCompileErr: false,
		},
		"output needs context appRevision": {
			workloadTemplate: `
output:{
	apiVersion: "apps/v1"
    kind: "Deployment"
	metadata: {
      name: context.name
      annotations: "revision.oam.dev": context.appRevision
    }
    spec: replicas: parameter.replicas
}
parameter: {
	replicas: *1 | int
	type: string
	host: string
}
`,
			params: map[string]interface{}{
				"replicas": 2,
				"type":     "ClusterIP",
				"host":     "example.com",
			},
			expectObj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{
						"name": "test", "annotations": map[string]interface{}{
							"revision.oam.dev": "myapp-v1",
						},
					}, "spec": map[string]interface{}{
						"replicas": int64(2),
					},
				},
			},
			hasCompileErr: false,
		},
		"output needs context replicas": {
			workloadTemplate: `
output:{
	apiVersion: "apps/v1"
    kind: "Deployment"
	metadata: {
      name: context.name
    }
    spec: replicas: parameter.replicas
}
parameter: {
	replicas: *1 | int
}
`,
			params: nil,
			expectObj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "test"},
					"spec": map[string]interface{}{
						"replicas": int64(1),
					},
				},
			},
			hasCompileErr: false,
		},
		"parameter type doesn't match will raise error": {
			workloadTemplate: `
output:{
	apiVersion: "apps/v1"
    kind: "Deployment"
	metadata: name: context.name
    spec: replicas: parameter.replicas
}
parameter: {
	replicas: *1 | int
	type: string
	host: string
}
`,
			params: map[string]interface{}{
				"replicas": "2",
				"type":     "ClusterIP",
				"host":     "example.com",
			},
			expectObj: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata":   map[string]interface{}{"name": "test"},
				"spec":       map[string]interface{}{"replicas": int64(2)},
			}},
			hasCompileErr: true,
		},
		"cluster version info": {
			workloadTemplate: `
output:{
  if context.clusterVersion.minor <  19 {
    apiVersion: "networking.k8s.io/v1beta1"
  }
  if context.clusterVersion.minor >= 19 {
    apiVersion: "networking.k8s.io/v1"
  }
  "kind":       "Ingress",
}
`,
			params: map[string]interface{}{},
			expectObj: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "networking.k8s.io/v1",
				"kind":       "Ingress",
			}},
		},
	}

	for _, v := range testCases {
		ctx := process.NewContext(process.ContextData{
			AppName:         "myapp",
			CompName:        "test",
			Namespace:       "default",
			AppRevisionName: "myapp-v1",
			ClusterVersion:  types.ClusterVersion{Minor: "19+"},
		})
		wt := NewWorkloadAbstractEngine("testWorkload", &packages.PackageDiscover{})
		err := wt.Complete(ctx, v.workloadTemplate, v.params)
		hasError := err != nil
		assert.Equal(t, v.hasCompileErr, hasError)
		if v.hasCompileErr {
			continue
		}
		base, assists := ctx.Output()
		assert.Equal(t, len(v.expAssObjs), len(assists))
		assert.NotNil(t, base)
		baseObj, err := base.Unstructured()
		assert.Equal(t, nil, err)
		assert.Equal(t, v.expectObj, baseObj)
		for _, ss := range assists {
			assert.Equal(t, AuxiliaryWorkload, ss.Type)
			got, err := ss.Ins.Unstructured()
			assert.NoError(t, err)
			assert.Equal(t, got, v.expAssObjs[ss.Name])
		}
	}

}

func TestTraitTemplateComplete(t *testing.T) {

	tds := map[string]struct {
		traitName     string
		traitTemplate string
		params        map[string]interface{}
		expWorkload   *unstructured.Unstructured
		expAssObjs    map[string]runtime.Object
		hasCompileErr bool
	}{
		"patch trait": {
			traitTemplate: `
patch: {
      // +patchKey=name
      spec: template: spec: containers: [parameter]
}

parameter: {
	name: string
	image: string
	command?: [...string]
}`,
			params: map[string]interface{}{
				"name":  "sidecar",
				"image": "metrics-agent:0.2",
			},
			expWorkload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"spec": map[string]interface{}{
						"replicas": int64(2),
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app.oam.dev/component": "test"}},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{"app.oam.dev/component": "test"},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{map[string]interface{}{
									"envFrom": []interface{}{map[string]interface{}{
										"configMapRef": map[string]interface{}{"name": "testgame-config"},
									}},
									"image": "website:0.1",
									"name":  "main",
									"ports": []interface{}{map[string]interface{}{"containerPort": int64(443)}}},
									map[string]interface{}{"image": "metrics-agent:0.2", "name": "sidecar"}}}}}},
			},
			expAssObjs: map[string]runtime.Object{
				"AuxiliaryWorkloadgameconfig": &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "testgame-config"}, "data": map[string]interface{}{"enemies": "enemies-data", "lives": "lives-data"}},
				},
			},
		},

		"patch trait with strategic merge": {
			traitTemplate: `
patch: {
      // +patchKey=name
      spec: template: spec: {
		// +patchStrategy=retainKeys
		containers: [{
			name:  "main"
			image: parameter.image
			ports: [{containerPort: parameter.port}]
			envFrom: [{
				configMapRef: name: context.name + "game-config"
			}]
			if parameter["command"] != _|_ {
				command: parameter.command
			}
	  }]	
	}
}

parameter: {
	image: string
	port: int
	command?: [...string]
}
`,
			params: map[string]interface{}{
				"image":   "website:0.2",
				"port":    8080,
				"command": []string{"server", "start"},
			},
			expWorkload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"spec": map[string]interface{}{
						"replicas": int64(2),
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app.oam.dev/component": "test"}},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{"app.oam.dev/component": "test"},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{map[string]interface{}{
									"envFrom": []interface{}{map[string]interface{}{
										"configMapRef": map[string]interface{}{"name": "testgame-config"},
									}},
									"image":   "website:0.2",
									"name":    "main",
									"command": []interface{}{"server", "start"},
									"ports":   []interface{}{map[string]interface{}{"containerPort": int64(8080)}}},
								}}}}},
			},
			expAssObjs: map[string]runtime.Object{
				"AuxiliaryWorkloadgameconfig": &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "testgame-config"}, "data": map[string]interface{}{"enemies": "enemies-data", "lives": "lives-data"}},
				},
			},
		},
		"patch trait with json merge patch": {
			traitTemplate: `
parameter: {...}
// +patchStrategy=jsonMergePatch
patch: parameter
`,
			params: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": 5,
					"template": map[string]interface{}{
						"spec": nil,
					},
				},
			},
			expWorkload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"spec": map[string]interface{}{
						"replicas": int64(5),
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app.oam.dev/component": "test"}},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{"app.oam.dev/component": "test"},
							}}}},
			},
			expAssObjs: map[string]runtime.Object{
				"AuxiliaryWorkloadgameconfig": &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "testgame-config"}, "data": map[string]interface{}{"enemies": "enemies-data", "lives": "lives-data"}},
				},
			},
		},
		"patch trait with json patch": {
			traitTemplate: `
parameter: {operations: [...{...}]}
// +patchStrategy=jsonPatch
patch: parameter
`,
			params: map[string]interface{}{
				"operations": []map[string]interface{}{
					{"op": "replace", "path": "/spec/replicas", "value": 5},
					{"op": "remove", "path": "/spec/template/spec"},
				},
			},
			expWorkload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"spec": map[string]interface{}{
						"replicas": int64(5),
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app.oam.dev/component": "test"}},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{"app.oam.dev/component": "test"},
							}}}},
			},
			expAssObjs: map[string]runtime.Object{
				"AuxiliaryWorkloadgameconfig": &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "testgame-config"}, "data": map[string]interface{}{"enemies": "enemies-data", "lives": "lives-data"}},
				},
			},
		},
		"patch trait with invalid json patch": {
			traitTemplate: `
parameter: {patch: [...{...}]}
// +patchStrategy=jsonPatch
patch: parameter
`,
			params: map[string]interface{}{
				"patch": []map[string]interface{}{
					{"op": "what", "path": "/spec/replicas", "value": 5},
				},
			},
			hasCompileErr: true,
		},
		"patch trait with replace": {
			traitTemplate: `
parameter: {
  name: string
  ports: [...int]
}
patch: spec: template: spec: {
  // +patchKey=name
  containers: [{
    name: parameter.name
    // +patchStrategy=replace
    ports: [for k in parameter.ports {containerPort: k}]
  }]
}
`,
			params: map[string]interface{}{
				"name":  "main",
				"ports": []int{80, 8443},
			},
			expWorkload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"spec": map[string]interface{}{
						"replicas": int64(2),
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app.oam.dev/component": "test"}},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{"app.oam.dev/component": "test"},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{map[string]interface{}{
									"envFrom": []interface{}{map[string]interface{}{
										"configMapRef": map[string]interface{}{"name": "testgame-config"},
									}},
									"image": "website:0.1",
									"name":  "main",
									"ports": []interface{}{
										map[string]interface{}{"containerPort": int64(80)},
										map[string]interface{}{"containerPort": int64(8443)},
									}},
								}}}}},
			},
			expAssObjs: map[string]runtime.Object{
				"AuxiliaryWorkloadgameconfig": &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "testgame-config"}, "data": map[string]interface{}{"enemies": "enemies-data", "lives": "lives-data"}},
				},
			},
		},
		"output trait": {
			traitTemplate: `
outputs: service: {
	apiVersion: "v1"
    kind: "Service"
	metadata: name: context.name
    spec: type: parameter.type
}
parameter: {
	type: string
}`,
			params: map[string]interface{}{
				"type": "ClusterIP",
			},
			expWorkload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"spec": map[string]interface{}{
						"replicas": int64(2),
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app.oam.dev/component": "test"}},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{"app.oam.dev/component": "test"},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{map[string]interface{}{
									"envFrom": []interface{}{map[string]interface{}{
										"configMapRef": map[string]interface{}{"name": "testgame-config"},
									}},
									"image": "website:0.1",
									"name":  "main",
									"ports": []interface{}{map[string]interface{}{"containerPort": int64(443)}}}}}}}},
			},
			traitName: "t1",
			expAssObjs: map[string]runtime.Object{
				"AuxiliaryWorkloadgameconfig": &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "testgame-config"}, "data": map[string]interface{}{"enemies": "enemies-data", "lives": "lives-data"}},
				},
				"t1service": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Service", "metadata": map[string]interface{}{"name": "test"}, "spec": map[string]interface{}{"type": "ClusterIP"}}},
			},
		},
		"outputs trait": {
			traitTemplate: `
outputs: service: {
	apiVersion: "v1"
    kind: "Service"
	metadata: name: context.name
    spec: type: parameter.type
}
outputs: ingress: {
	apiVersion: "extensions/v1beta1"
    kind: "Ingress"
	metadata: name: context.name
    spec: rules: [{host: parameter.host}]
}
parameter: {
	type: string
	host: string
}`,
			params: map[string]interface{}{
				"type": "ClusterIP",
				"host": "example.com",
			},
			expWorkload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"spec": map[string]interface{}{
						"replicas": int64(2),
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app.oam.dev/component": "test"}},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{"app.oam.dev/component": "test"},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{map[string]interface{}{
									"envFrom": []interface{}{map[string]interface{}{
										"configMapRef": map[string]interface{}{"name": "testgame-config"},
									}},
									"image": "website:0.1",
									"name":  "main",
									"ports": []interface{}{map[string]interface{}{"containerPort": int64(443)}}}}}}}},
			},
			traitName: "t2",
			expAssObjs: map[string]runtime.Object{
				"AuxiliaryWorkloadgameconfig": &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "testgame-config"}, "data": map[string]interface{}{"enemies": "enemies-data", "lives": "lives-data"}},
				},
				"t2service": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Service", "metadata": map[string]interface{}{"name": "test"}, "spec": map[string]interface{}{"type": "ClusterIP"}}},
				"t2ingress": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "extensions/v1beta1", "kind": "Ingress", "metadata": map[string]interface{}{"name": "test"}, "spec": map[string]interface{}{"rules": []interface{}{map[string]interface{}{
					"host": "example.com",
				}}}}},
			},
		},
		"outputs trait with context appRevision": {
			traitTemplate: `
outputs: service: {
	apiVersion: "v1"
    kind: "Service"
    metadata: {
      name: context.name
      annotations: "revision.oam.dev": context.appRevision
    }
    spec: type: parameter.type
}
outputs: ingress: {
	apiVersion: "extensions/v1beta1"
    kind: "Ingress"
	metadata: name: context.name
    spec: rules: [{host: parameter.host}]
}
parameter: {
	type: string
	host: string
}`,
			params: map[string]interface{}{
				"type": "ClusterIP",
				"host": "example.com",
			},
			expWorkload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"spec": map[string]interface{}{
						"replicas": int64(2),
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app.oam.dev/component": "test"}},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{"app.oam.dev/component": "test"},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{map[string]interface{}{
									"envFrom": []interface{}{map[string]interface{}{
										"configMapRef": map[string]interface{}{"name": "testgame-config"},
									}},
									"image": "website:0.1",
									"name":  "main",
									"ports": []interface{}{map[string]interface{}{"containerPort": int64(443)}}}}}}}},
			},
			traitName: "t2",
			expAssObjs: map[string]runtime.Object{
				"AuxiliaryWorkloadgameconfig": &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "testgame-config"}, "data": map[string]interface{}{"enemies": "enemies-data", "lives": "lives-data"}},
				},
				"t2service": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Service", "metadata": map[string]interface{}{"name": "test", "annotations": map[string]interface{}{
					"revision.oam.dev": "myapp-v1",
				}}, "spec": map[string]interface{}{"type": "ClusterIP"}}},
				"t2ingress": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "extensions/v1beta1", "kind": "Ingress", "metadata": map[string]interface{}{"name": "test"}, "spec": map[string]interface{}{"rules": []interface{}{map[string]interface{}{
					"host": "example.com",
				}}}}},
			},
		},
		"simple data passing": {
			traitTemplate: `
      parameter: {
        domain: string
        path: string
        exposePort: int
      }
      // trait template can have multiple outputs in one trait
      outputs: service: {
        apiVersion: "v1"
        kind: "Service"
        spec: {
          selector:
            app: context.name
          ports: [
            {
              port: parameter.exposePort
              targetPort: context.output.spec.template.spec.containers[0].ports[0].containerPort
            }
          ]
        }
      }
      outputs: ingress: {
        apiVersion: "networking.k8s.io/v1beta1"
        kind: "Ingress"
        metadata:
          name: context.name
          labels: config: context.outputs.gameconfig.data.enemies
        spec: {
          rules: [{
            host: parameter.domain
            http: {
              paths: [{
                  path: parameter.path
                  backend: {
                    serviceName: context.name
                    servicePort: parameter.exposePort
                  }
              }]
            }
          }]
        }
      }`,
			params: map[string]interface{}{
				"domain":     "example.com",
				"path":       "ping",
				"exposePort": 1080,
			},
			expWorkload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"spec": map[string]interface{}{
						"replicas": int64(2),
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app.oam.dev/component": "test"}},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{"app.oam.dev/component": "test"},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{map[string]interface{}{
									"envFrom": []interface{}{map[string]interface{}{
										"configMapRef": map[string]interface{}{"name": "testgame-config"},
									}},
									"image": "website:0.1",
									"name":  "main",
									"ports": []interface{}{map[string]interface{}{"containerPort": int64(443)}}}}}}}},
			},
			traitName: "t3",
			expAssObjs: map[string]runtime.Object{
				"AuxiliaryWorkloadgameconfig": &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "testgame-config"}, "data": map[string]interface{}{"enemies": "enemies-data", "lives": "lives-data"}},
				},
				"t3service": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Service", "spec": map[string]interface{}{"ports": []interface{}{map[string]interface{}{"port": int64(1080), "targetPort": int64(443)}}, "selector": map[string]interface{}{"app": "test"}}}},
				"t3ingress": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "networking.k8s.io/v1beta1", "kind": "Ingress", "labels": map[string]interface{}{"config": "enemies-data"}, "metadata": map[string]interface{}{"name": "test"}, "spec": map[string]interface{}{"rules": []interface{}{map[string]interface{}{"host": "example.com", "http": map[string]interface{}{"paths": []interface{}{map[string]interface{}{"backend": map[string]interface{}{"serviceName": "test", "servicePort": int64(1080)}, "path": "ping"}}}}}}}},
			},
		},
		"outputs trait with schema": {
			traitTemplate: `
#Service:{
  apiVersion: string
  kind: string
}
#Ingress:{
  apiVersion: string
  kind: string
}
outputs:{
  service: #Service
  ingress: #Ingress
}
outputs: service: {
	apiVersion: "v1"
    kind: "Service"
}
outputs: ingress: {
	apiVersion: "extensions/v1beta1"
    kind: "Ingress"
}
parameter: {
	type: string
	host: string
}`,
			params: map[string]interface{}{
				"type": "ClusterIP",
				"host": "example.com",
			},
			expWorkload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"spec": map[string]interface{}{
						"replicas": int64(2),
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app.oam.dev/component": "test"}},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{"app.oam.dev/component": "test"},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{map[string]interface{}{
									"envFrom": []interface{}{map[string]interface{}{
										"configMapRef": map[string]interface{}{"name": "testgame-config"},
									}},
									"image": "website:0.1",
									"name":  "main",
									"ports": []interface{}{map[string]interface{}{"containerPort": int64(443)}}}}}}}},
			},
			traitName: "t2",
			expAssObjs: map[string]runtime.Object{
				"AuxiliaryWorkloadgameconfig": &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "testgame-config"}, "data": map[string]interface{}{"enemies": "enemies-data", "lives": "lives-data"}},
				},
				"t2service": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Service"}},
				"t2ingress": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "extensions/v1beta1", "kind": "Ingress"}},
			},
		},
		"outputs trait with no params": {
			traitTemplate: `
outputs: hpa: {
	apiVersion: "autoscaling/v2beta2"
	kind:       "HorizontalPodAutoscaler"
	metadata: name: context.name
	spec: {
		minReplicas: parameter.min
		maxReplicas: parameter.max
	}
}
parameter: {
	min:     *1 | int
	max:     *10 | int
}`,
			params: nil,
			expWorkload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"spec": map[string]interface{}{
						"replicas": int64(2),
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app.oam.dev/component": "test"}},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{"app.oam.dev/component": "test"},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{map[string]interface{}{
									"envFrom": []interface{}{map[string]interface{}{
										"configMapRef": map[string]interface{}{"name": "testgame-config"},
									}},
									"image": "website:0.1",
									"name":  "main",
									"ports": []interface{}{map[string]interface{}{"containerPort": int64(443)}}}}}}}},
			},
			traitName: "t2",
			expAssObjs: map[string]runtime.Object{
				"AuxiliaryWorkloadgameconfig": &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "testgame-config"}, "data": map[string]interface{}{"enemies": "enemies-data", "lives": "lives-data"}},
				},
				"t2hpa": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "autoscaling/v2beta2", "kind": "HorizontalPodAutoscaler",
					"metadata": map[string]interface{}{"name": "test"},
					"spec":     map[string]interface{}{"maxReplicas": int64(10), "minReplicas": int64(1)}}},
			},
		},
		"parameter type doesn't match will raise error": {
			traitTemplate: `
      parameter: {
        exposePort: int
      }
      // trait template can have multiple outputs in one trait
      outputs: service: {
        apiVersion: "v1"
        kind: "Service"
        spec: {
          selector:
            app: context.name
          ports: [
            {
              port: parameter.exposePort
              targetPort: parameter.exposePort
            }
          ]
        }
      }
`,
			params: map[string]interface{}{
				"exposePort": "1080",
			},
			hasCompileErr: true,
		},

		"trait patch trait": {
			traitTemplate: `
patchOutputs: {
	gameconfig: {
		metadata: annotations: parameter
	}
}

parameter: [string]: string`,
			params: map[string]interface{}{
				"patch-by": "trait",
			},
			expWorkload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"spec": map[string]interface{}{
						"replicas": int64(2),
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app.oam.dev/component": "test"}},
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{"app.oam.dev/component": "test"},
							},
							"spec": map[string]interface{}{
								"containers": []interface{}{map[string]interface{}{
									"envFrom": []interface{}{map[string]interface{}{
										"configMapRef": map[string]interface{}{"name": "testgame-config"},
									}},
									"image": "website:0.1",
									"name":  "main",
									"ports": []interface{}{map[string]interface{}{"containerPort": int64(443)}}}}}}}},
			},
			expAssObjs: map[string]runtime.Object{
				"AuxiliaryWorkloadgameconfig": &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "testgame-config", "annotations": map[string]interface{}{"patch-by": "trait"}}, "data": map[string]interface{}{"enemies": "enemies-data", "lives": "lives-data"}},
				},
			},
		},

		// errors
		"invalid template(space-separated labels) will raise error": {
			traitTemplate: `
a b: c`,
			params:        map[string]interface{}{},
			hasCompileErr: true,
		},
		"reference a non-existent variable will raise error": {
			traitTemplate: `
patch: {
	metadata: name: none
}

parameter: [string]: string`,
			params:        map[string]interface{}{},
			hasCompileErr: true,
		},
		"out-of-scope variables in patch will raise error": {
			traitTemplate: `
patchOutputs: {
	x : "out of scope"
	gameconfig: {
		metadata: name: x
	}
}

parameter: [string]: string`,
			params:        map[string]interface{}{},
			hasCompileErr: true,
		},
		"using the wrong keyword in the parameter will raise error": {
			traitTemplate: `
patch: {
	metadata: annotations: parameter
}

parameter: [string]: string`,
			params: map[string]interface{}{
				"wrong-keyword": 5,
			},
			hasCompileErr: true,
		},
		"using errs": {
			traitTemplate: `
errs: parameter.errs
parameter: { errs: [...string] }`,
			params: map[string]interface{}{
				"errs": []string{"has error"},
			},
			hasCompileErr: true,
		},
	}

	for cassinfo, v := range tds {
		baseTemplate := `
	output: {
      	apiVersion: "apps/v1"
      	kind:       "Deployment"
      	spec: {
      		selector: matchLabels: {
      			"app.oam.dev/component": context.name
      		}
			replicas: parameter.replicas
      		template: {
      			metadata: labels: {
      				"app.oam.dev/component": context.name
      			}
      			spec: {
      				containers: [{
      					name:  "main"
      					image: parameter.image
						ports: [{containerPort: parameter.port}]
      					envFrom: [{
      						configMapRef: name: context.name + "game-config"
      					}]
      					if parameter["cmd"] != _|_ {
      						command: parameter.cmd
      					}
      				}]
      			}
      		}
      	}
	}

	outputs: gameconfig: {
      	apiVersion: "v1"
      	kind:       "ConfigMap"
      	metadata: {
      		name: context.name + "game-config"
      	}
      	data: {
      		enemies: parameter.enemies
      		lives:   parameter.lives
      	}
	}

	parameter: {
      	// +usage=Which image would you like to use for your service
      	// +short=i
      	image: *"website:0.1" | string
      	// +usage=Commands to run in the container
      	cmd?: [...string]
		replicas: *1 | int
      	lives:   string
      	enemies: string
        port: int
	}

`
		ctx := process.NewContext(process.ContextData{
			AppName:         "myapp",
			CompName:        "test",
			Namespace:       "default",
			AppRevisionName: "myapp-v1",
		})
		wt := NewWorkloadAbstractEngine("-", &packages.PackageDiscover{})
		if err := wt.Complete(ctx, baseTemplate, map[string]interface{}{
			"replicas": 2,
			"enemies":  "enemies-data",
			"lives":    "lives-data",
			"port":     443,
		}); err != nil {
			t.Error(err)
			return
		}
		td := NewTraitAbstractEngine(v.traitName, &packages.PackageDiscover{})
		r := require.New(t)
		err := td.Complete(ctx, v.traitTemplate, v.params)
		if v.hasCompileErr {
			r.Error(err, cassinfo)
			continue
		}
		r.NoError(err, cassinfo)
		base, assists := ctx.Output()
		r.Equal(len(v.expAssObjs), len(assists), cassinfo)
		r.NotNil(base)
		obj, err := base.Unstructured()
		r.NoError(err)
		r.Equal(v.expWorkload, obj, cassinfo)
		for _, ss := range assists {
			got, err := ss.Ins.Unstructured()
			r.NoError(err, cassinfo)
			r.Equal(v.expAssObjs[ss.Type+ss.Name], got, "case %s , type: %s name: %s, got: %s", cassinfo, ss.Type, ss.Name, got)
		}
	}
}

func TestWorkloadTemplateCompleteRenderOrder(t *testing.T) {
	testcases := map[string]struct {
		template string
		order    []struct {
			name    string
			content string
		}
	}{
		"dict-order": {
			template: `
output: {
	kind: "Deployment"
}

outputs: configMap :{
	name: "test-configMap"
}

outputs: ingress :{
	name: "test-ingress"
}

outputs: service :{
	name: "test-service"
}
`,
			order: []struct {
				name    string
				content string
			}{{
				name:    "configMap",
				content: "name: \"test-configMap\"\n",
			}, {
				name:    "ingress",
				content: "name: \"test-ingress\"\n",
			}, {
				name:    "service",
				content: "name: \"test-service\"\n",
			}},
		},
		"non-dict-order": {
			template: `
output: {
	name: "base"
}
outputs: route :{
	name: "test-route"
}

outputs: service :{
	name: "test-service"
}
`,
			order: []struct {
				name    string
				content string
			}{{
				name:    "route",
				content: "name: \"test-route\"\n",
			}, {
				name:    "service",
				content: "name: \"test-service\"\n",
			}},
		},
	}
	for k, v := range testcases {
		wd := NewWorkloadAbstractEngine(k, &packages.PackageDiscover{})
		ctx := process.NewContext(process.ContextData{
			AppName:         "myapp",
			CompName:        k,
			Namespace:       "default",
			AppRevisionName: "myapp-v1",
		})
		err := wd.Complete(ctx, v.template, map[string]interface{}{})
		assert.NoError(t, err)
		_, assists := ctx.Output()
		for i, ss := range assists {
			assert.Equal(t, ss.Name, v.order[i].name)
			s, err := ss.Ins.String()
			assert.NoError(t, err)
			assert.Equal(t, s, v.order[i].content)
		}
	}
}

func TestTraitTemplateCompleteRenderOrder(t *testing.T) {
	testcases := map[string]struct {
		template string
		order    []struct {
			name    string
			content string
		}
	}{
		"dict-order": {
			template: `
outputs: abc :{
	name: "test-abc"
}

outputs: def :{
	name: "test-def"
}

outputs: ghi :{
	name: "test-ghi"
}
`,
			order: []struct {
				name    string
				content string
			}{{
				name:    "abc",
				content: "name: \"test-abc\"\n",
			}, {
				name:    "def",
				content: "name: \"test-def\"\n",
			}, {
				name:    "ghi",
				content: "name: \"test-ghi\"\n",
			}},
		},
		"non-dict-order": {
			template: `
outputs: zyx :{
	name: "test-zyx"
}

outputs: lmn :{
	name: "test-lmn"
}

outputs: abc :{
	name: "test-abc"
}
`,
			order: []struct {
				name    string
				content string
			}{{
				name:    "zyx",
				content: "name: \"test-zyx\"\n",
			}, {
				name:    "lmn",
				content: "name: \"test-lmn\"\n",
			}, {
				name:    "abc",
				content: "name: \"test-abc\"\n",
			}},
		},
	}
	for k, v := range testcases {
		td := NewTraitAbstractEngine(k, &packages.PackageDiscover{})
		ctx := process.NewContext(process.ContextData{
			AppName:         "myapp",
			CompName:        k,
			Namespace:       "default",
			AppRevisionName: "myapp-v1",
		})
		err := td.Complete(ctx, v.template, map[string]interface{}{})
		assert.NoError(t, err)
		_, assists := ctx.Output()
		for i, ss := range assists {
			assert.Equal(t, ss.Name, v.order[i].name)
			s, err := ss.Ins.String()
			assert.NoError(t, err)
			assert.Equal(t, s, v.order[i].content)
		}
	}
}

func TestCheckHealth(t *testing.T) {
	cases := map[string]struct {
		tpContext  map[string]interface{}
		healthTemp string
		parameter  interface{}
		exp        bool
	}{
		"normal-equal": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"status": map[string]interface{}{
						"readyReplicas": 4,
						"replicas":      4,
					},
				},
			},
			healthTemp: "isHealth:  context.output.status.readyReplicas == context.output.status.replicas",
			parameter:  nil,
			exp:        true,
		},
		"normal-false": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"status": map[string]interface{}{
						"readyReplicas": 4,
						"replicas":      5,
					},
				},
			},
			healthTemp: "isHealth: context.output.status.readyReplicas == context.output.status.replicas",
			parameter:  nil,
			exp:        false,
		},
		"array-case-equal": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{"status": "True"},
						},
					},
				},
			},
			healthTemp: `isHealth: context.output.status.conditions[0].status == "True"`,
			parameter:  nil,
			exp:        true,
		},
		"parameter-false": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"status": map[string]interface{}{
						"replicas": 4,
					},
				},
				"outputs": map[string]interface{}{
					"my": map[string]interface{}{
						"status": map[string]interface{}{
							"readyReplicas": 4,
						},
					},
				},
			},
			healthTemp: "isHealth: context.outputs[parameter.res].status.readyReplicas == context.output.status.replicas",
			parameter: map[string]string{
				"res": "my",
			},
			exp: true,
		},
	}
	for message, ca := range cases {
		healthy, err := checkHealth(ca.tpContext, ca.healthTemp, ca.parameter)
		assert.NoError(t, err, message)
		assert.Equal(t, ca.exp, healthy, message)
	}
}

func TestGetStatus(t *testing.T) {
	cases := map[string]struct {
		tpContext  map[string]interface{}
		parameter  interface{}
		statusTemp string
		expMessage string
	}{
		"field-with-array-and-outputs": {
			tpContext: map[string]interface{}{
				"outputs": map[string]interface{}{
					"service": map[string]interface{}{
						"spec": map[string]interface{}{
							"type":      "NodePort",
							"clusterIP": "10.0.0.1",
							"ports": []interface{}{
								map[string]interface{}{
									"port": 80,
								},
							},
						},
					},
					"ingress": map[string]interface{}{
						"rules": []interface{}{
							map[string]interface{}{
								"host": "example.com",
							},
						},
					},
				},
			},
			statusTemp: `message: "type: " + context.outputs.service.spec.type + " clusterIP:" + context.outputs.service.spec.clusterIP + " ports:" + "\(context.outputs.service.spec.ports[0].port)" + " domain:" + context.outputs.ingress.rules[0].host`,
			expMessage: "type: NodePort clusterIP:10.0.0.1 ports:80 domain:example.com",
		},
		"complex status": {
			tpContext: map[string]interface{}{
				"outputs": map[string]interface{}{
					"ingress": map[string]interface{}{
						"spec": map[string]interface{}{
							"rules": []interface{}{
								map[string]interface{}{
									"host": "example.com",
								},
							},
						},
						"status": map[string]interface{}{
							"loadBalancer": map[string]interface{}{
								"ingress": []interface{}{
									map[string]interface{}{
										"ip": "10.0.0.1",
									},
								},
							},
						},
					},
				},
			},
			statusTemp: `if len(context.outputs.ingress.status.loadBalancer.ingress) > 0 {
	message: "Visiting URL: " + context.outputs.ingress.spec.rules[0].host + ", IP: " + context.outputs.ingress.status.loadBalancer.ingress[0].ip
}
if len(context.outputs.ingress.status.loadBalancer.ingress) == 0 {
	message: "No loadBalancer found, visiting by using 'vela port-forward " + context.appName + " --route'\n"
}`,
			expMessage: "Visiting URL: example.com, IP: 10.0.0.1",
		},
		"status use parameter field": {
			tpContext: map[string]interface{}{
				"outputs": map[string]interface{}{
					"test-name": map[string]interface{}{
						"spec": map[string]interface{}{
							"type":      "NodePort",
							"clusterIP": "10.0.0.1",
							"ports": []interface{}{
								map[string]interface{}{
									"port": 80,
								},
							},
						},
					},
				},
			},
			parameter: map[string]interface{}{
				"configInfo": map[string]string{
					"name": "test-name",
				},
			},
			statusTemp: `message: parameter.configInfo.name + ".type: " + context.outputs["\(parameter.configInfo.name)"].spec.type`,
			expMessage: "test-name.type: NodePort",
		},
		"import package in template": {
			tpContext: map[string]interface{}{
				"outputs": map[string]interface{}{
					"service": map[string]interface{}{
						"spec": map[string]interface{}{
							"type":      "NodePort",
							"clusterIP": "10.0.0.1",
							"ports": []interface{}{
								map[string]interface{}{
									"port": 80,
								},
							},
						},
					},
					"ingress": map[string]interface{}{
						"rules": []interface{}{
							map[string]interface{}{
								"host": "example.com",
							},
						},
					},
				},
			},
			statusTemp: `import "strconv"
      message: "ports: " + strconv.FormatInt(context.outputs.service.spec.ports[0].port,10)`,
			expMessage: "ports: 80",
		},
	}
	for message, ca := range cases {
		gotMessage, err := getStatusMessage(&packages.PackageDiscover{}, ca.tpContext, ca.statusTemp, ca.parameter)
		assert.NoError(t, err, message)
		assert.Equal(t, ca.expMessage, gotMessage, message)
	}
}

func TestTraitPatchSingleOutput(t *testing.T) {
	baseTemplate := `
	output: {
      	apiVersion: "apps/v1"
      	kind:       "Deployment"
      	spec: selector: matchLabels: "app.oam.dev/component": context.name
	}

	outputs: gameconfig: {
      	apiVersion: "v1"
      	kind:       "ConfigMap"
      	metadata: name: context.name + "game-config"
      	data: {}
	}

	outputs: sideconfig: {
      	apiVersion: "v1"
      	kind:       "ConfigMap"
      	metadata: name: context.name + "side-config"
      	data: {}
	}

	parameter: {}
`
	traitTemplate := `
	patchOutputs: sideconfig: data: key: "val"
	parameter: {}
`
	ctx := process.NewContext(process.ContextData{
		AppName:         "myapp",
		CompName:        "test",
		Namespace:       "default",
		AppRevisionName: "myapp-v1",
	})
	wt := NewWorkloadAbstractEngine("-", &packages.PackageDiscover{})
	if err := wt.Complete(ctx, baseTemplate, map[string]interface{}{}); err != nil {
		t.Error(err)
		return
	}
	td := NewTraitAbstractEngine("single-patch", &packages.PackageDiscover{})
	r := require.New(t)
	err := td.Complete(ctx, traitTemplate, map[string]string{})
	r.NoError(err)
	base, assists := ctx.Output()
	r.NotNil(base)
	r.Equal(2, len(assists))
	got, err := assists[1].Ins.Unstructured()
	r.NoError(err)
	val, ok, err := unstructured.NestedString(got.Object, "data", "key")
	r.NoError(err)
	r.True(ok)
	r.Equal("val", val)
}

func TestTraitCompleteErrorCases(t *testing.T) {
	cases := map[string]struct {
		ctx       wfprocess.Context
		traitName string
		template  string
		params    map[string]interface{}
		err       string
	}{
		"patch trait": {
			ctx: process.NewContext(process.ContextData{}),
			template: `
patch: {
      // +patchKey=name
      spec: template: spec: containers: [parameter]
}
parameter: {
	name: string
	image: string
	command?: [...string]
}`,
			err: "patch trait patch trait into an invalid workload",
		},
	}
	for k, v := range cases {
		td := NewTraitAbstractEngine(k, &packages.PackageDiscover{})
		err := td.Complete(v.ctx, v.template, v.params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), v.err)
	}
}
