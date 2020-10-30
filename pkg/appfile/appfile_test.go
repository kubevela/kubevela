package appfile

import (
	"os"
	"testing"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

func TestRenderOAM(t *testing.T) {
	yamlOneService := `name: myapp
services:
  express-server:
    image: oamdev/testapp:v1
    cmd: ["node", "server.js"]
    route:
      domain: example.com
      http:
        "/": 8080
`
	yamlTwoServices := yamlOneService + `
  mongodb:
    type: backend
    image: bitnami/mongodb:3.6.20
    cmd: ["mongodb"]
`
	yamlNoImage := `name: myapp
services:
  bad-server:
    build:
      docker:
        file: Dockerfile
    cmd: ["node", "server.js"]
`

	yamlWithConfig := `name: myapp
services:
  express-server:
    type: withconfig
    image: oamdev/testapp:v1
    cmd: ["node", "server.js"]
    route:
      domain: example.com
      http:
        "/": 8080
    config: test
`

	templateWebservice := `parameter: #webservice
#webservice: {
  cmd: [...string]
  image: string
}

output: {
  apiVersion: "test.oam.dev/v1"
  kind: "WebService"
  metadata: {
    name: context.name
  }
  spec: {
    image: parameter.image
    command: parameter.cmd
  }
}
`
	templateBackend := `parameter: #backend
#backend: {
  cmd: [...string]
  image: string
}

output: {
  apiVersion: "test.oam.dev/v1"
  kind: "Worker"
  metadata: {
    name: context.name
  }
  spec: {
    image: parameter.image
    command: parameter.cmd
  }
}`
	templateWithConfig := `parameter: #withconfig
#withconfig: {
  cmd: [...string]
  image: string
}

output: {
  apiVersion: "test.oam.dev/v1"
  kind: "WebService"
  metadata: {
    name: context.name
  }
  spec: {
    image: parameter.image
    command: parameter.cmd
    env: context.config
  }
}
`
	templateRoute := `parameter: #route
#route: {
  domain: string
  http: [string]: int
}

// trait template can have multiple outputs and they are all traits
outputs: service: {
  apiVersion: "v1"
  kind: "Service"
  metadata:
    name: context.name
  spec: {
    selector:
      app: context.name
    ports: [
      for k, v in parameter.http {
        port: v
        targetPort: v
      }
    ]
  }
}

outputs: ingress: {
  apiVersion: "networking.k8s.io/v1beta1"
  kind: "Ingress"
  spec: {
    rules: [{
      host: parameter.domain
      http: {
        paths: [
          for k, v in parameter.http {
            path: k
            backend: {
              serviceName: context.name
              servicePort: v
            }
          }
        ]
      }
    }]
  }
}
`
	ac1 := &v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp",
			Namespace: "default",
		},
		Spec: v1alpha2.ApplicationConfigurationSpec{
			Components: []v1alpha2.ApplicationConfigurationComponent{{
				ComponentName: "express-server",
				Scopes: []v1alpha2.ComponentScope{{
					ScopeReference: v1alpha1.TypedReference{
						APIVersion: "core.oam.dev/v1alpha2",
						Kind:       "HealthScope",
						Name:       "myapp-default-health",
					},
				}},
				Traits: []v1alpha2.ComponentTrait{{
					Trait: runtime.RawExtension{
						Object: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "Service",
								"metadata": map[string]interface{}{
									"name": "express-server",
								},
								"spec": map[string]interface{}{
									"selector": map[string]interface{}{
										"app": "express-server",
									},
									"ports": []interface{}{
										map[string]interface{}{
											"port":       float64(8080),
											"targetPort": float64(8080),
										},
									},
								},
							},
						},
					},
				}, {
					Trait: runtime.RawExtension{
						Object: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "networking.k8s.io/v1beta1",
								"kind":       "Ingress",
								"spec": map[string]interface{}{
									"rules": []interface{}{
										map[string]interface{}{
											"http": map[string]interface{}{
												"paths": []interface{}{
													map[string]interface{}{
														"path": "/",
														"backend": map[string]interface{}{
															"serviceName": "express-server",
															"servicePort": float64(8080),
														},
													},
												},
											},
											"host": "example.com",
										},
									},
								},
							},
						},
					},
				}},
			}},
		},
	}
	ac2 := ac1.DeepCopy()
	ac2.Spec.Components = append(ac2.Spec.Components, v1alpha2.ApplicationConfigurationComponent{
		ComponentName: "mongodb",
		Traits:        []v1alpha2.ComponentTrait{},
		Scopes: []v1alpha2.ComponentScope{{
			ScopeReference: v1alpha1.TypedReference{
				APIVersion: "core.oam.dev/v1alpha2",
				Kind:       "HealthScope",
				Name:       "myapp-default-health",
			},
		}},
	})

	comp1 := &v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "express-server",
			Namespace: "default",
		},
		Spec: v1alpha2.ComponentSpec{
			Workload: runtime.RawExtension{
				Object: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "test.oam.dev/v1",
						"kind":       "WebService",
						"metadata": map[string]interface{}{
							"name": "express-server",
							"labels": map[string]interface{}{
								"workload.oam.dev/type": "webservice",
							},
						},
						"spec": map[string]interface{}{
							"image":   "oamdev/testapp:v1",
							"command": []interface{}{"node", "server.js"},
						},
					},
				},
			},
		},
	}

	comp2 := &v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mongodb",
			Namespace: "default",
		},
		Spec: v1alpha2.ComponentSpec{
			Workload: runtime.RawExtension{
				Object: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "test.oam.dev/v1",
						"kind":       "Worker",
						"metadata": map[string]interface{}{
							"name": "mongodb",
							"labels": map[string]interface{}{
								"workload.oam.dev/type": "backend",
							},
						},
						"spec": map[string]interface{}{
							"image":   "bitnami/mongodb:3.6.20",
							"command": []interface{}{"mongodb"},
						},
					},
				},
			},
		},
	}

	compWithConfig := comp1.DeepCopy()
	fakeConfigData2 := []map[string]string{{
		"name":  "test",
		"value": "test-value",
	}}
	// for deepCopy. Otherwise deepcopy will panic in SetNestedField.
	fakeConfigData := []interface{}{map[string]interface{}{
		"name":  "test",
		"value": "test-value",
	}}
	if err := unstructured.SetNestedField(
		compWithConfig.Spec.Workload.Object.(*unstructured.Unstructured).UnstructuredContent(),
		fakeConfigData, "spec", "env"); err != nil {
		t.Fatal(err)
	}
	compWithConfig.Spec.Workload.Object.(*unstructured.Unstructured).SetLabels(
		map[string]string{"workload.oam.dev/type": "withconfig"})

	type args struct {
		appfileData       string
		workloadTemplates map[string]string
		traitTemplates    map[string]string
	}
	type want struct {
		components []*v1alpha2.Component
		appConfig  *v1alpha2.ApplicationConfiguration
		err        error
	}
	cases := map[string]struct {
		args args
		want want
	}{
		"one service should generate one component and one appconfig": {
			args: args{
				appfileData: yamlOneService,
				workloadTemplates: map[string]string{
					"webservice": templateWebservice,
				},
				traitTemplates: map[string]string{
					"route": templateRoute,
				},
			},
			want: want{
				appConfig:  ac1,
				components: []*v1alpha2.Component{comp1},
			},
		},
		"two services should generate two components and one appconfig": {
			args: args{
				appfileData: yamlTwoServices,
				workloadTemplates: map[string]string{
					"webservice": templateWebservice,
					"backend":    templateBackend,
				},
				traitTemplates: map[string]string{
					"route": templateRoute,
				},
			},
			want: want{
				appConfig:  ac2,
				components: []*v1alpha2.Component{comp1, comp2},
			},
		},
		"no image should fail": {
			args: args{
				appfileData: yamlNoImage,
			},
			want: want{
				err: ErrImageNotDefined,
			},
		},
		"config data should be set": {
			args: args{
				appfileData: yamlWithConfig,
				workloadTemplates: map[string]string{
					"withconfig": templateWithConfig,
				},
				traitTemplates: map[string]string{
					"route": templateRoute,
				},
			},
			want: want{
				appConfig:  ac1,
				components: []*v1alpha2.Component{compWithConfig},
			},
		},
	}

	io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	for caseName, c := range cases {
		t.Run(caseName, func(t *testing.T) {
			app := NewAppFile()
			app.configGetter = &fakeConfigGetter{
				Data: fakeConfigData2,
			}
			err := yaml.Unmarshal([]byte(c.args.appfileData), app)
			if err != nil {
				t.Fatal(err)
			}
			tm := template.NewFakeTemplateManager()
			for k, v := range c.args.traitTemplates {
				tm.Templates[k] = &template.Template{
					Captype: types.TypeTrait,
					Raw:     v,
				}
			}
			for k, v := range c.args.workloadTemplates {
				tm.Templates[k] = &template.Template{
					Captype: types.TypeWorkload,
					Raw:     v,
				}
			}

			comps, ac, _, err := app.RenderOAM("default", io, tm, false)
			if err != nil {
				assert.Equal(t, c.want.err, err)
				return
			}

			assert.Equal(t, ac.ObjectMeta, c.want.appConfig.ObjectMeta)

			for _, cp1 := range c.want.appConfig.Spec.Components {
				found := false
				for _, cp2 := range ac.Spec.Components {
					if cp1.ComponentName != cp2.ComponentName {
						continue
					}
					assert.Equal(t, cp1, cp2)
					found = true
					break
				}
				if !found {
					t.Errorf("ac component (%s) not found", cp1.ComponentName)
				}
			}
			for _, cp1 := range c.want.components {
				found := false
				for _, cp2 := range comps {
					if cp1.Name != cp2.Name {
						continue
					}
					assert.Equal(t, cp1.Spec.Workload.Object, cp2.Spec.Workload.Object)
					found = true
					break
				}
				if !found {
					t.Errorf("component (%s) not found", cp1.Name)
				}
			}
		})
	}
}

func TestAddWorkloadTypeLabel(t *testing.T) {
	tests := map[string]struct {
		comps    []*v1alpha2.Component
		services map[string]Service
		expect   []*v1alpha2.Component
	}{
		"empty case": {
			comps:    []*v1alpha2.Component{},
			services: map[string]Service{},
			expect:   []*v1alpha2.Component{},
		},
		"add type to labels normal case": {
			comps: []*v1alpha2.Component{
				{
					ObjectMeta: v1.ObjectMeta{Name: "mycomp"},
					Spec:       v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &unstructured.Unstructured{Object: map[string]interface{}{}}}},
				},
			},
			services: map[string]Service{
				"mycomp": {"type": "kubewatch"},
			},
			expect: []*v1alpha2.Component{
				{
					ObjectMeta: v1.ObjectMeta{Name: "mycomp"},
					Spec: v1alpha2.ComponentSpec{
						Workload: runtime.RawExtension{
							Object: &unstructured.Unstructured{Object: map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"workload.oam.dev/type": "kubewatch",
									}}}}},
					},
				},
			},
		},
	}
	for key, ca := range tests {
		addWorkloadTypeLabel(ca.comps, ca.services)
		assert.Equal(t, ca.expect, ca.comps, key)
	}
}
