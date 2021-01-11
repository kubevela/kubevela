package builder

import (
	"testing"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/parser"
)

func TestBuild(t *testing.T) {

	// TestApp is test data
	var TestApp = &parser.Appfile{
		Name: "test",
		Services: []*parser.Workload{
			{
				Name: "myweb",
				Type: "worker",
				Params: map[string]interface{}{
					"image": "busybox",
					"cmd":   []interface{}{"sleep", "1000"},
				},
				Scopes: []parser.Scope{
					{Name: "test-scope", GVK: schema.GroupVersionKind{
						Group:   "core.oam.dev",
						Version: "v1alpha2",
						Kind:    "HealthScope",
					}},
				},
				Template: `
      output: {
        apiVersion: "apps/v1"
      	kind:       "Deployment"
      	spec: {
      		selector: matchLabels: {
      			"app.oam.dev/component": context.name
      		}
      
      		template: {
      			metadata: labels: {
      				"app.oam.dev/component": context.name
      			}
      
      			spec: {
      				containers: [{
      					name:  context.name
      					image: parameter.image
      
      					if parameter["cmd"] != _|_ {
      						command: parameter.cmd
      					}
      				}]
      			}
      		}
      
      		selector:
      			matchLabels:
      				"app.oam.dev/component": context.name
      	}
      }
      
      parameter: {
      	// +usage=Which image would you like to use for your service
      	// +short=i
      	image: string
      
      	cmd?: [...string]
      }`,
				Traits: []*parser.Trait{
					{
						Name: "scaler",
						Params: map[string]interface{}{
							"replicas": float64(10),
						},
						Template: `
      output: {
      	apiVersion: "core.oam.dev/v1alpha2"
      	kind:       "ManualScalerTrait"
      	spec: {
      		replicaCount: parameter.replicas
      	}
      }
      parameter: {
      	//+short=r
      	replicas: *1 | int
      }
`,
					},
				},
			},
		},
	}

	ac, components, err := Build("default", TestApp, nil)
	if err != nil {
		t.Error(err)
	}

	expectAppConfig := &v1alpha2.ApplicationConfiguration{
		TypeMeta: v1.TypeMeta{
			Kind:       "ApplicationConfiguration",
			APIVersion: "core.oam.dev/v1alpha2",
		}, ObjectMeta: v1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Labels:    map[string]string{"application.oam.dev": "test"},
		},
		Spec: v1alpha2.ApplicationConfigurationSpec{
			Components: []v1alpha2.ApplicationConfigurationComponent{
				{
					ComponentName: "myweb",
					Scopes: []v1alpha2.ComponentScope{
						{
							ScopeReference: v1alpha1.TypedReference{
								APIVersion: "core.oam.dev/v1alpha2",
								Kind:       "HealthScope",
								Name:       "test-scope",
							},
						},
					},
					Traits: []v1alpha2.ComponentTrait{
						{
							Trait: runtime.RawExtension{
								Object: &unstructured.Unstructured{
									Object: map[string]interface{}{
										"apiVersion": "core.oam.dev/v1alpha2",
										"kind":       "ManualScalerTrait",
										"metadata": map[string]interface{}{
											"labels": map[string]interface{}{
												"trait.oam.dev/type": "scaler",
											},
										},
										"spec": map[string]interface{}{"replicaCount": int64(10)},
									},
								},
							}},
					},
				},
			},
		},
	}
	assert.Equal(t, expectAppConfig, ac)

	expectComponent := &v1alpha2.Component{
		TypeMeta: v1.TypeMeta{
			Kind:       "Component",
			APIVersion: "core.oam.dev/v1alpha2",
		}, ObjectMeta: v1.ObjectMeta{
			Name:      "myweb",
			Namespace: "default",
			Labels:    map[string]string{"application.oam.dev": "test"},
		}, Spec: v1alpha2.ComponentSpec{
			Workload: runtime.RawExtension{
				Object: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"workload.oam.dev/type": "worker",
							},
						},
						"spec": map[string]interface{}{
							"selector": map[string]interface{}{
								"matchLabels": map[string]interface{}{
									"app.oam.dev/component": "myweb"}},
							"template": map[string]interface{}{
								"metadata": map[string]interface{}{"labels": map[string]interface{}{"app.oam.dev/component": "myweb"}},
								"spec": map[string]interface{}{
									"containers": []interface{}{
										map[string]interface{}{
											"command": []interface{}{"sleep", "1000"},
											"image":   "busybox",
											"name":    "myweb"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	assert.Equal(t, 1, len(components), " built components' length must be 1")
	assert.Equal(t, expectComponent, components[0])
}
