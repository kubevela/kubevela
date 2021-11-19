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

package api

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/appfile/template"
)

func TestBuildOAMApplication2(t *testing.T) {
	expectNs := "test-ns"

	tm := template.NewFakeTemplateManager()
	tm.Templates = map[string]*template.Template{
		"containerWorkload": {
			Captype: types.TypeWorkload,
			Raw:     `{parameters : {image: string} }`,
		},
		"scaler": {
			Captype: types.TypeTrait,
			Raw:     `{parameters : {relicas: int} }`,
		},
	}

	testCases := []struct {
		appFile   *AppFile
		expectApp *v1beta1.Application
	}{
		{
			appFile: &AppFile{
				Name: "test",
				Services: map[string]Service{
					"webapp": map[string]interface{}{
						"type":  "containerWorkload",
						"image": "busybox",
					},
				},
			},
			expectApp: &v1beta1.Application{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "core.oam.dev/v1beta1",
				}, ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "webapp",
							Type: "containerWorkload",
							Properties: &runtime.RawExtension{
								Raw: []byte("{\"image\":\"busybox\"}"),
							},
							Scopes: map[string]string{"healthscopes.core.oam.dev": "test-default-health"},
						},
					},
				},
			},
		},
		{
			appFile: &AppFile{
				Name: "test",
				Services: map[string]Service{
					"webapp": map[string]interface{}{
						"type":  "containerWorkload",
						"image": "busybox",
						"scaler": map[string]interface{}{
							"replicas": 10,
						},
					},
				},
			},
			expectApp: &v1beta1.Application{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Application",
					APIVersion: "core.oam.dev/v1beta1",
				}, ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{
						{
							Name: "webapp",
							Type: "containerWorkload",
							Properties: &runtime.RawExtension{
								Raw: []byte("{\"image\":\"busybox\"}"),
							},
							Scopes: map[string]string{"healthscopes.core.oam.dev": "test-default-health"},
							Traits: []common.ApplicationTrait{
								{
									Type: "scaler",
									Properties: &runtime.RawExtension{
										Raw: []byte("{\"replicas\":10}"),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tcase := range testCases {
		tcase.expectApp.Namespace = expectNs
		o, _, err := tcase.appFile.BuildOAMApplication(expectNs, cmdutil.IOStreams{
			In:  os.Stdin,
			Out: os.Stdout,
		}, tm, false)
		assert.NoError(t, err)
		assert.Equal(t, tcase.expectApp, o)
	}
}

func TestBuildOAMApplication(t *testing.T) {
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
	ac1 := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp",
			Namespace: "default",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{
				Type:   "webservice",
				Name:   "express-server",
				Scopes: map[string]string{"healthscopes.core.oam.dev": "myapp-default-health"},
				Properties: &runtime.RawExtension{
					Raw: []byte(`{"image": "oamdev/testapp:v1", "cmd": ["node", "server.js"]}`),
				},
				Traits: []common.ApplicationTrait{{
					Type: "route",
					Properties: &runtime.RawExtension{
						Raw: []byte(`{"domain": "example.com", "http":{"/": 8080}}`),
					},
				},
				},
			}},
		},
	}
	ac2 := ac1.DeepCopy()
	ac2.Spec.Components = append(ac2.Spec.Components, common.ApplicationComponent{
		Name: "mongodb",
		Type: "backend",
		Properties: &runtime.RawExtension{
			Raw: []byte(`{"image":"bitnami/mongodb:3.6.20","cmd": ["mongodb"]}`),
		},
		Traits: []common.ApplicationTrait{},
		Scopes: map[string]string{"healthscopes.core.oam.dev": "myapp-default-health"},
	})

	ac3 := ac1.DeepCopy()
	ac3.Spec.Components[0].Type = "withconfig"

	// TODO application 那边补测试:
	// 2. 1对多的情况，多对1 的情况

	health := &v1alpha2.HealthScope{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.HealthScopeGroupVersionKind.GroupVersion().String(),
			Kind:       v1alpha2.HealthScopeKind,
		},
	}
	health.Name = FormatDefaultHealthScopeName("myapp")
	health.Namespace = "default"
	health.Spec.WorkloadReferences = make([]corev1.ObjectReference, 0)
	type args struct {
		appfileData       string
		workloadTemplates map[string]string
		traitTemplates    map[string]string
	}
	type want struct {
		objs []oam.Object
		app  *v1beta1.Application
		err  error
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
				app:  ac1,
				objs: []oam.Object{health},
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
				app:  ac2,
				objs: []oam.Object{health},
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
	}

	io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	for caseName, c := range cases {
		t.Run(caseName, func(t *testing.T) {

			app := NewAppFile()
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

			application, objects, err := app.BuildOAMApplication("default", io, tm, false)
			if c.want.err != nil {
				assert.Equal(t, c.want.err, err)
				return
			}
			assert.Equal(t, c.want.app.ObjectMeta, application.ObjectMeta)
			for _, comp := range application.Spec.Components {
				var found bool
				for idx, expcomp := range c.want.app.Spec.Components {
					if comp.Name != expcomp.Name {
						continue
					}
					found = true
					assert.Equal(t, comp.Type, c.want.app.Spec.Components[idx].Type)
					assert.Equal(t, comp.Name, c.want.app.Spec.Components[idx].Name)
					assert.Equal(t, comp.Scopes, c.want.app.Spec.Components[idx].Scopes)

					got, err := util.RawExtension2Map(comp.Properties)
					assert.NoError(t, err)
					exp, err := util.RawExtension2Map(c.want.app.Spec.Components[idx].Properties)
					assert.NoError(t, err)
					assert.Equal(t, exp, got)
					for tidx, tr := range comp.Traits {
						assert.Equal(t, tr.Type, c.want.app.Spec.Components[idx].Traits[tidx].Type)

						got, err := util.RawExtension2Map(tr.Properties)
						assert.NoError(t, err)
						exp, err := util.RawExtension2Map(c.want.app.Spec.Components[idx].Traits[tidx].Properties)
						assert.NoError(t, err)
						assert.Equal(t, exp, got)
					}
				}
				assert.Equal(t, true, found, "no component found for %s", comp.Name)
			}
			for idx, v := range objects {
				assert.Equal(t, c.want.objs[idx], v)
			}

		})
	}
}
