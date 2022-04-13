/*
Copyright 2022 The KubeVela Authors.

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

package debug

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	yamlUtil "sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
)

func TestSetContext(t *testing.T) {
	r := require.New(t)
	cli := newCliForTest(t, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: GenerateContextName("test", "step1"),
		},
		Data: map[string]string{
			"debug": "test",
		},
	})
	debugCtx := NewContext(cli, &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}, "step1")
	v, err := value.NewValue(`
test: test
`, nil, "")
	r.NoError(err)
	err = debugCtx.Set(v)
	r.NoError(err)
}

func newCliForTest(t *testing.T, wfCm *corev1.ConfigMap) *test.MockClient {
	r := require.New(t)
	return &test.MockClient{
		MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			o, ok := obj.(*corev1.ConfigMap)
			if ok {
				switch key.Name {
				case "app-v1":
					var cm corev1.ConfigMap
					testCaseJson, err := yamlUtil.YAMLToJSON([]byte(testCaseYaml))
					r.NoError(err)
					err = json.Unmarshal(testCaseJson, &cm)
					r.NoError(err)
					*o = cm
					return nil
				case GenerateContextName("test", "step1"):
					if wfCm != nil {
						*o = *wfCm
						return nil
					}
				}
			}
			return kerrors.NewNotFound(corev1.Resource("configMap"), key.Name)
		},
		MockUpdate: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			o, ok := obj.(*corev1.ConfigMap)
			if ok {
				if wfCm == nil {
					return kerrors.NewNotFound(corev1.Resource("configMap"), o.Name)
				}
				*wfCm = *o
			}
			return nil
		},
	}
}

var (
	testCaseYaml = `apiVersion: v1
data:
  debug: ''
kind: ConfigMap
metadata:
  name: app-v1
`
)
