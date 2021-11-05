/*
 Copyright 2021. The KubeVela Authors.

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

package envbinding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

func Test_EnvBindApp_GenerateConfiguredApplication(t *testing.T) {
	testcases := []struct {
		baseApp     *v1beta1.Application
		envName     string
		envPatch    v1alpha1.EnvPatch
		expectedApp *v1beta1.Application
	}{{
		baseApp: baseApp,
		envName: "prod",
		envPatch: v1alpha1.EnvPatch{
			Components: []common.ApplicationComponent{{
				Name: "express-server",
				Type: "webservice",
				Properties: util.Object2RawExtension(map[string]interface{}{
					"image": "busybox",
				}),
				Traits: []common.ApplicationTrait{{
					Type: "ingress-1-20",
					Properties: util.Object2RawExtension(map[string]interface{}{
						"domain": "newTestsvc.example.com",
					}),
				}},
			}},
		},
		expectedApp: &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1beta1",
				Kind:       "Application",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{
					Name: "express-server",
					Type: "webservice",
					Properties: util.Object2RawExtension(map[string]interface{}{
						"image": "busybox",
						"port":  8000,
					}),
					Traits: []common.ApplicationTrait{{
						Type: "ingress-1-20",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"domain": "newTestsvc.example.com",
							"http": map[string]interface{}{
								"/": 8000,
							},
						}),
					}},
				}},
			},
		},
	}, {
		baseApp: baseApp,
		envName: "prod",
		envPatch: v1alpha1.EnvPatch{
			Components: []common.ApplicationComponent{{
				Name: "express-server",
				Type: "webservice",
				Traits: []common.ApplicationTrait{{
					Type: "labels",
					Properties: util.Object2RawExtension(map[string]interface{}{
						"test": "label",
					}),
				}},
			}, {
				Name: "new-server",
				Type: "worker",
				Properties: util.Object2RawExtension(map[string]interface{}{
					"image": "busybox",
					"cmd":   []string{"sleep", "1000"},
				}),
				Traits: []common.ApplicationTrait{{
					Type: "labels",
					Properties: util.Object2RawExtension(map[string]interface{}{
						"test": "label",
					}),
				}},
			}},
		},
		expectedApp: &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1beta1",
				Kind:       "Application",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{
					Name: "express-server",
					Type: "webservice",
					Properties: util.Object2RawExtension(map[string]interface{}{
						"image": "crccheck/hello-world",
						"port":  8000,
					}),
					Traits: []common.ApplicationTrait{{
						Type: "ingress-1-20",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"domain": "testsvc.example.com",
							"http": map[string]interface{}{
								"/": 8000,
							},
						}),
					}, {
						Type: "labels",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"test": "label",
						}),
					}},
				}, {
					Name: "new-server",
					Type: "worker",
					Properties: util.Object2RawExtension(map[string]interface{}{
						"image": "busybox",
						"cmd":   []string{"sleep", "1000"},
					}),
					Traits: []common.ApplicationTrait{{
						Type: "labels",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"test": "label",
						}),
					}},
				}},
			},
		},
	}}

	for _, testcase := range testcases {
		app, err := PatchApplication(testcase.baseApp, &testcase.envPatch, nil)
		assert.NoError(t, err)
		assert.Equal(t, app, testcase.expectedApp)
	}
}

var baseApp = &v1beta1.Application{
	TypeMeta: metav1.TypeMeta{
		APIVersion: "v1beta1",
		Kind:       "Application",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "test",
	},
	Spec: v1beta1.ApplicationSpec{
		Components: []common.ApplicationComponent{{
			Name: "express-server",
			Type: "webservice",
			Properties: util.Object2RawExtension(map[string]interface{}{
				"image": "crccheck/hello-world",
				"port":  8000,
			}),
			Traits: []common.ApplicationTrait{{
				Type: "ingress-1-20",
				Properties: util.Object2RawExtension(map[string]interface{}{
					"domain": "testsvc.example.com",
					"http": map[string]interface{}{
						"/": 8000,
					},
				}),
			}},
		}},
	},
}
