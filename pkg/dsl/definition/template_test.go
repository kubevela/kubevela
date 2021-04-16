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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/pkg/dsl/process"
)

func TestWorkloadTemplateComplete(t *testing.T) {
	testCases := map[string]struct {
		workloadTemplate string
		params           map[string]interface{}
		expectObj        runtime.Object
		expAssObjs       map[string]runtime.Object
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
		},
	}

	for _, v := range testCases {
		ctx := process.NewContext("default", "test", "myapp", "myapp-v1")
		wt := NewWorkloadAbstractEngine("testworkload", &PackageDiscover{})
		assert.NoError(t, wt.Complete(ctx, v.workloadTemplate, v.params))
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
		ctx := process.NewContext("default", "test", "myapp", "myapp-v1")
		wt := NewWorkloadAbstractEngine("-", &PackageDiscover{})
		if err := wt.Complete(ctx, baseTemplate, map[string]interface{}{
			"replicas": 2,
			"enemies":  "enemies-data",
			"lives":    "lives-data",
			"port":     443,
		}); err != nil {
			t.Error(err)
			return
		}
		td := NewTraitAbstractEngine(v.traitName, &PackageDiscover{})
		assert.NoError(t, td.Complete(ctx, v.traitTemplate, v.params))
		base, assists := ctx.Output()
		assert.Equal(t, len(v.expAssObjs), len(assists), cassinfo)
		assert.NotNil(t, base)
		obj, err := base.Unstructured()
		assert.NoError(t, err)
		assert.Equal(t, v.expWorkload, obj, cassinfo)
		for _, ss := range assists {
			got, err := ss.Ins.Unstructured()
			assert.NoError(t, err, cassinfo)
			assert.Equal(t, v.expAssObjs[ss.Type+ss.Name], got, "case %s , type: %s name: %s", cassinfo, ss.Type, ss.Name)
		}
	}
}

func TestCheckHealth(t *testing.T) {
	cases := map[string]struct {
		tpContext  map[string]interface{}
		healthTemp string
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
			exp:        true,
		},
	}
	for message, ca := range cases {
		healthy, err := checkHealth(ca.tpContext, ca.healthTemp)
		assert.NoError(t, err, message)
		assert.Equal(t, ca.exp, healthy, message)
	}
}

func TestGetStatus(t *testing.T) {
	cases := map[string]struct {
		tpContext  map[string]interface{}
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
	}
	for message, ca := range cases {
		gotMessage, err := getStatusMessage(ca.tpContext, ca.statusTemp)
		assert.NoError(t, err, message)
		assert.Equal(t, ca.expMessage, gotMessage, message)
	}
}
