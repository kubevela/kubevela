package driver

import (
	"testing"

	"github.com/bmizerany/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
)

func TestObject(t *testing.T) {
	expectNs := "test-ns"

	tm := template.NewFakeTemplateManager()
	tm.Templates = map[string]*template.Template{
		"containerWorkload": &template.Template{
			Captype: types.TypeWorkload,
			Raw:     `{parameters : {image: string} }`,
		},
		"scaler": &template.Template{
			Captype: types.TypeTrait,
			Raw:     `{parameters : {relicas: int} }`,
		},
	}

	testCases := []struct {
		appFile   *appfile.AppFile
		expectApp *v1alpha2.Application
	}{
		{
			appFile: &appfile.AppFile{
				Name: "test",
				Services: map[string]appfile.Service{
					"webapp": map[string]interface{}{
						"type":  "containerWorkload",
						"image": "busybox",
					},
				},
			},
			expectApp: &v1alpha2.Application{
				TypeMeta: v1.TypeMeta{
					Kind:       "Application",
					APIVersion: "core.oam.dev/v1alpha2",
				}, ObjectMeta: v1.ObjectMeta{
					Name: "test",
				},
				Spec: v1alpha2.ApplicationSpec{
					Components: []v1alpha2.ApplicationComponent{
						{
							Name:         "webapp",
							WorkloadType: "containerWorkload",
							Settings: runtime.RawExtension{
								Raw: []byte("{\"image\":\"busybox\"}"),
							},
						},
					},
				},
			},
		},
		{
			appFile: &appfile.AppFile{
				Name: "test",
				Services: map[string]appfile.Service{
					"webapp": map[string]interface{}{
						"type":  "containerWorkload",
						"image": "busybox",
						"scaler": map[string]interface{}{
							"replicas": 10,
						},
					},
				},
			},
			expectApp: &v1alpha2.Application{
				TypeMeta: v1.TypeMeta{
					Kind:       "Application",
					APIVersion: "core.oam.dev/v1alpha2",
				}, ObjectMeta: v1.ObjectMeta{
					Name: "test",
				},
				Spec: v1alpha2.ApplicationSpec{
					Components: []v1alpha2.ApplicationComponent{
						{
							Name:         "webapp",
							WorkloadType: "containerWorkload",
							Settings: runtime.RawExtension{
								Raw: []byte("{\"image\":\"busybox\"}"),
							},
							Traits: []v1alpha2.ApplicationTrait{
								{
									Name: "scaler",
									Properties: runtime.RawExtension{
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
		app := NewApplication(tcase.appFile, tm)
		o, _, err := app.Object(expectNs)
		assert.Equal(t, nil, err)
		assert.Equal(t, tcase.expectApp, o)
	}
}
