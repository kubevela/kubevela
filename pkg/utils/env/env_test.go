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

package env

import (
	"fmt"
	"testing"

	"gotest.tools/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"

	"github.com/oam-dev/kubevela/apis/types"
)

func TestEnv(t *testing.T) {
	testCases := []struct {
		envName        string
		namespace      string
		wantComponents []common2.ApplicationComponent
		labels         map[string]string
	}{
		{
			envName:   "test-env",
			namespace: "test-env-ns",
			wantComponents: []common2.ApplicationComponent{
				{
					Name: "test-env-ns",
					Type: RawType,
					Properties: util.Object2RawExtension(map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "Namespace",
						"metadata":   map[string]string{"name": "test-env-ns"},
					})},
			},
			labels: map[string]string{IndicatingLabel: "test-env"},
		},

		{
			envName:        "test-env-default",
			namespace:      "default",
			wantComponents: []common2.ApplicationComponent{},
			labels:         map[string]string{IndicatingLabel: "test-env-default"},
		},
	}
	baseApp := v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1beta1.ApplicationKind,
			APIVersion: v1beta1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "",
			Namespace: types.DefaultKubeVelaNS,
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common2.ApplicationComponent{}}}
	for _, c := range testCases {
		env := &types.EnvMeta{
			Name:      c.envName,
			Namespace: c.namespace,
		}
		rightApp := baseApp.DeepCopy()
		rightApp.ObjectMeta.Name = fmt.Sprintf(AppNameSchema, c.envName)
		rightApp.ObjectMeta.Labels = c.labels
		rightApp.Spec.Components = c.wantComponents

		generatedApp := env2App(env)
		assert.DeepEqual(t, generatedApp, rightApp)

		generatedEnv, err := app2Env(rightApp)
		assert.NilError(t, err)
		assert.DeepEqual(t, generatedEnv, env)
	}
}
