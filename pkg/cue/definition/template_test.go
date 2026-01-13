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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	wfprocess "github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam/util"
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
		wt := NewWorkloadAbstractEngine("testWorkload")
		err := wt.Complete(ctx, v.workloadTemplate, v.params /* use default validation */)
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
        apiVersion: "networking.k8s.io/v1"
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
                  pathType: "Prefix"
                  backend: {
                    service: {
                      name: context.name
                      port: {
                        number: parameter.exposePort
                      }
                    }
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
				"t3ingress": &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "networking.k8s.io/v1", "kind": "Ingress", "labels": map[string]interface{}{"config": "enemies-data"}, "metadata": map[string]interface{}{"name": "test"}, "spec": map[string]interface{}{"rules": []interface{}{map[string]interface{}{"host": "example.com", "http": map[string]interface{}{"paths": []interface{}{map[string]interface{}{"path": "ping", "pathType": "Prefix", "backend": map[string]interface{}{"service": map[string]interface{}{"name": "test", "port": map[string]interface{}{"number": int64(1080)}}}}}}}}}}},
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
		wt := NewWorkloadAbstractEngine("-")
		if err := wt.Complete(ctx, baseTemplate, map[string]interface{}{
			"replicas": 2,
			"enemies":  "enemies-data",
			"lives":    "lives-data",
			"port":     443,
		}); err != nil {
			t.Error(err)
			return
		}
		td := NewTraitAbstractEngine(v.traitName)
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
		wd := NewWorkloadAbstractEngine(k)
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
		td := NewTraitAbstractEngine(k)
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
	wt := NewWorkloadAbstractEngine("-")
	if err := wt.Complete(ctx, baseTemplate, map[string]interface{}{}); err != nil {
		t.Error(err)
		return
	}
	td := NewTraitAbstractEngine("single-patch")
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
		td := NewTraitAbstractEngine(k)
		err := td.Complete(v.ctx, v.template, v.params)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), v.err)
	}
}

func TestValidationErrorFormatting(t *testing.T) {
	testCases := map[string]struct {
		name       string
		template   string
		params     map[string]interface{}
		isWorkload bool
		wantErr    string
	}{
		"workload validation with parameter errors": {
			name: "my-workload",
			template: `
parameter: {
	name: string
	replicas: int & >=1
}
output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: name: parameter.name
	spec: replicas: parameter.replicas
}`,
			params: map[string]interface{}{
				"name":     123,
				"replicas": -1,
			},
			isWorkload: true,
			wantErr:    "validation failed for workload my-workload:\n\nParameter errors:\n  parameter.name: conflicting values string and 123 (mismatched types string and int)\n  parameter.replicas: invalid value -1 (out of bound >=1)",
		},
		"trait validation with parameter errors": {
			name: "my-trait",
			template: `
parameter: {
	port: int & >=1 & <=65535
	protocol: "TCP" | "UDP"
}
outputs: service: {
	apiVersion: "v1"
	kind: "Service"
	spec: {
		ports: [{
			port: parameter.port
			protocol: parameter.protocol
		}]
	}
}`,
			params: map[string]interface{}{
				"port":     70000,
				"protocol": "INVALID",
			},
			isWorkload: false,
			wantErr:    "validation failed for trait my-trait:\n\nParameter errors:\n  parameter.port: invalid value 70000 (out of bound <=65535)\n  parameter.protocol: 2 errors in empty disjunction:\n  parameter.protocol: conflicting values \"TCP\" and \"INVALID\"\n  parameter.protocol: conflicting values \"UDP\" and \"INVALID\"",
		},
		"mixed parameter and template errors": {
			name: "test-workload",
			template: `
parameter: {
	image: string
}
output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	spec: {
		replicas: "invalid"
		template: spec: containers: [{
			image: parameter.image
		}]
	}
}`,
			params: map[string]interface{}{
				"image": 123,
			},
			isWorkload: true,
			wantErr:    "validation failed for workload test-workload:\n\nParameter errors:\n  parameter.image: conflicting values string and 123 (mismatched types string and int)",
		},
	}

	for testName, tc := range testCases {
		t.Run(testName, func(t *testing.T) {
			ctx := process.NewContext(process.ContextData{
				AppName:  "test-app",
				CompName: "test-comp",
			})

			var err error
			if tc.isWorkload {
				wd := NewWorkloadAbstractEngine(tc.name)
				err = wd.Complete(ctx, tc.template, tc.params)
			} else {
				td := NewTraitAbstractEngine(tc.name)
				err = td.Complete(ctx, tc.template, tc.params)
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)

			errStr := err.Error()
			if strings.Contains(tc.template, "parameter:") {
				assert.True(t,
					strings.Contains(errStr, "Parameter errors:") ||
						strings.Contains(errStr, "Template errors:"),
					"Error should contain grouped error sections")
			}
		})
	}
}

func TestWorkloadGetTemplateContext(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	workload := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "test-workload",
				"namespace": "default",
			},
		},
	}
	auxSvc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "test-aux-svc",
				"namespace": "default",
			},
		},
	}

	baseCtx := process.NewContext(process.ContextData{
		AppName:         "myapp",
		CompName:        "test",
		Namespace:       "default",
		AppRevisionName: "myapp-v1",
	})

	workloadTemplate := `
output: {
	apiVersion: "apps/v1"
    kind: "Deployment"
	metadata: {
		name: "test-workload"
		namespace: "default"
	}
}
outputs: service: {
	apiVersion: "v1"
    kind: "Service"
	metadata: {
		name: "test-aux-svc"
		namespace: "default"
	}
}
`
	wt := NewWorkloadAbstractEngine("testWorkload")
	err := wt.Complete(baseCtx, workloadTemplate, nil)
	require.NoError(t, err)

	testCases := map[string]struct {
		reason    string
		cli       client.Client
		ctx       wfprocess.Context
		wantErr   bool
		checkFunc func(t *testing.T, templateContext map[string]interface{})
	}{
		"successfully get template context": {
			reason: "Should successfully get the template context with both output and outputs.",
			cli:    fake.NewClientBuilder().WithScheme(scheme).WithObjects(workload, auxSvc).Build(),
			ctx:    baseCtx,
			checkFunc: func(t *testing.T, templateContext map[string]interface{}) {
				require.NotNil(t, templateContext)
				output, ok := templateContext[OutputFieldName]
				require.True(t, ok)
				outputMap, ok := output.(map[string]interface{})
				require.True(t, ok)
				require.Equal(t, "test-workload", outputMap["metadata"].(map[string]interface{})["name"])

				outputs, ok := templateContext[OutputsFieldName]
				require.True(t, ok)
				outputsMap, ok := outputs.(map[string]interface{})
				require.True(t, ok)
				svc, ok := outputsMap["service"]
				require.True(t, ok)
				svcMap, ok := svc.(map[string]interface{})
				require.True(t, ok)
				require.Equal(t, "test-aux-svc", svcMap["metadata"].(map[string]interface{})["name"])
			},
		},
		"resource not found": {
			reason:  "Should return an error when a resource is not found in the cluster.",
			cli:     fake.NewClientBuilder().WithScheme(scheme).Build(),
			ctx:     baseCtx,
			wantErr: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			wd := &workloadDef{def: def{name: "test"}}
			accessor := util.NewApplicationResourceNamespaceAccessor("default", "")
			templateContext, err := wd.GetTemplateContext(tc.ctx, tc.cli, accessor)

			if tc.wantErr {
				require.Error(t, err, tc.reason)
			} else {
				require.NoError(t, err, tc.reason)
				if tc.checkFunc != nil {
					tc.checkFunc(t, templateContext)
				}
			}
		})
	}
}

func TestTraitGetTemplateContext(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	traitOutput := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-trait-output",
				"namespace": "default",
			},
		},
	}

	traitName := "my-trait"
	baseCtx := process.NewContext(process.ContextData{
		AppName:         "myapp",
		CompName:        "test",
		Namespace:       "default",
		AppRevisionName: "myapp-v1",
	})

	traitTemplate := `
outputs: myconfig: {
	apiVersion: "v1"
    kind: "ConfigMap"
	metadata: {
		name: "test-trait-output"
		namespace: "default"
	}
}
`
	td := NewTraitAbstractEngine(traitName)
	err := td.Complete(baseCtx, traitTemplate, nil)
	require.NoError(t, err)

	emptyCtx := process.NewContext(process.ContextData{
		AppName:         "myapp",
		CompName:        "test",
		Namespace:       "default",
		AppRevisionName: "myapp-v1",
	})

	testCases := map[string]struct {
		reason    string
		cli       client.Client
		ctx       wfprocess.Context
		traitName string
		wantErr   bool
		checkFunc func(t *testing.T, templateContext map[string]interface{})
	}{
		"successfully get template context for trait": {
			reason:    "Should successfully get the template context for a trait with outputs.",
			cli:       fake.NewClientBuilder().WithScheme(scheme).WithObjects(traitOutput).Build(),
			ctx:       baseCtx,
			traitName: traitName,
			checkFunc: func(t *testing.T, templateContext map[string]interface{}) {
				require.NotNil(t, templateContext)
				outputs, ok := templateContext[OutputsFieldName]
				require.True(t, ok)
				outputsMap, ok := outputs.(map[string]interface{})
				require.True(t, ok)
				cm, ok := outputsMap["myconfig"]
				require.True(t, ok)
				cmMap, ok := cm.(map[string]interface{})
				require.True(t, ok)
				require.Equal(t, "test-trait-output", cmMap["metadata"].(map[string]interface{})["name"])
			},
		},
		"trait resource not found": {
			reason:    "Should return an error when a trait's output resource is not found.",
			cli:       fake.NewClientBuilder().WithScheme(scheme).Build(),
			ctx:       baseCtx,
			traitName: traitName,
			wantErr:   true,
		},
		"trait with no outputs": {
			reason:    "Should successfully get a context for a trait that produces no outputs.",
			cli:       fake.NewClientBuilder().WithScheme(scheme).Build(),
			ctx:       emptyCtx,
			traitName: traitName,
			checkFunc: func(t *testing.T, templateContext map[string]interface{}) {
				require.NotNil(t, templateContext)
				_, ok := templateContext[OutputsFieldName]
				require.False(t, ok)
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			traitDef := &traitDef{def: def{name: tc.traitName}}
			accessor := util.NewApplicationResourceNamespaceAccessor("default", "")
			templateContext, err := traitDef.GetTemplateContext(tc.ctx, tc.cli, accessor)

			if tc.wantErr {
				require.Error(t, err, tc.reason)
			} else {
				require.NoError(t, err, tc.reason)
				if tc.checkFunc != nil {
					tc.checkFunc(t, templateContext)
				}
			}
		})
	}
}

func TestGetCommonLabels(t *testing.T) {
	type want struct {
		labels map[string]string
	}
	cases := map[string]struct {
		reason string
		input  map[string]string
		want   want
	}{
		"TestConvert": {
			reason: "Test that context labels are correctly converted to OAM labels",
			input: map[string]string{
				process.ContextAppName:     "my-app",
				process.ContextName:        "my-comp",
				process.ContextAppRevision: "v1",
				process.ContextReplicaKey:  "rep-key",
				"other-label":              "other-value",
			},
			want: want{
				labels: map[string]string{
					"app.oam.dev/name":        "my-app",
					"app.oam.dev/component":   "my-comp",
					"app.oam.dev/appRevision": "v1",
					"app.oam.dev/replicaKey":  "rep-key",
				},
			},
		},
		"TestEmpty": {
			reason: "Test that an empty input map results in an empty output map",
			input:  map[string]string{},
			want: want{
				labels: map[string]string{},
			},
		},
		"TestNoConvert": {
			reason: "Test that labels with no OAM equivalent are ignored",
			input: map[string]string{
				"other-label": "other-value",
			},
			want: want{
				labels: map[string]string{},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			got := GetCommonLabels(tc.input)
			r.Equal(tc.want.labels, got, tc.reason)
		})
	}
}

func TestGetBaseContextLabels(t *testing.T) {
	type want struct {
		labels map[string]string
	}
	cases := map[string]struct {
		reason string
		ctx    wfprocess.Context
		want   want
	}{
		"TestWithAppNameAndRevision": {
			reason: "Test that app name and revision are added to the base context labels",
			ctx: process.NewContext(process.ContextData{
				AppName:         "my-app",
				AppRevisionName: "v1",
				CompName:        "my-comp",
			}),
			want: want{
				labels: map[string]string{
					process.ContextAppName:     "my-app",
					process.ContextAppRevision: "v1",
					process.ContextName:        "my-comp",
				},
			},
		},
		"TestWithoutAppNameAndRevision": {
			reason: "Test that the base context labels are returned when app name and revision are missing",
			ctx: process.NewContext(process.ContextData{
				CompName: "my-comp",
			}),
			want: want{
				labels: map[string]string{
					process.ContextAppName:     "",
					process.ContextAppRevision: "",
					process.ContextName:        "my-comp",
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			got := GetBaseContextLabels(tc.ctx)
			r.Equal(tc.want.labels, got, tc.reason)
		})
	}
}
