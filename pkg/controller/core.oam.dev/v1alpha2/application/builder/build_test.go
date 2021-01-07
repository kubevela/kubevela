package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/parser"
)

func TestBuild(t *testing.T) {
	ac, componets, err := Build("default", parser.TestExceptApp, nil)
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
	assert.Equal(t, 1, len(componets), " built components' length must be 1")
	assert.Equal(t, expectComponent, componets[0])
}
