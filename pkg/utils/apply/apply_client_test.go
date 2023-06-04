/*
Copyright 2023 The KubeVela Authors.

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

package apply

import (
	"context"
	"testing"

	"github.com/kubevela/pkg/util/jsonutil"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestApplyClient(t *testing.T) {
	cli := applyClient{fake.NewClientBuilder().Build()}
	deploy, err := jsonutil.AsType[unstructured.Unstructured](&appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: pointer.Int32(1)},
		Status:     appsv1.DeploymentStatus{Replicas: 1},
	})
	require.NoError(t, err)
	_ctx := context.Background()
	require.NoError(t, cli.Create(_ctx, deploy))
	require.Equal(t, int64(1), deploy.Object["spec"].(map[string]interface{})["replicas"])
	require.Equal(t, int64(1), deploy.Object["status"].(map[string]interface{})["replicas"])

	deploy.Object["spec"].(map[string]interface{})["replicas"] = 3
	deploy.Object["status"].(map[string]interface{})["replicas"] = 3
	require.NoError(t, cli.Update(_ctx, deploy))

	_deploy := &unstructured.Unstructured{Object: map[string]interface{}{}}
	_deploy.SetAPIVersion("apps/v1")
	_deploy.SetKind("Deployment")
	require.NoError(t, cli.Get(_ctx, types.NamespacedName{Namespace: "default", Name: "test"}, _deploy))
	require.Equal(t, int64(3), _deploy.Object["spec"].(map[string]interface{})["replicas"])
	require.Equal(t, int64(3), _deploy.Object["status"].(map[string]interface{})["replicas"])

	p := client.RawPatch(types.JSONPatchType, []byte(`[{"op":"replace","path":"/spec/replicas","value":5},{"op":"replace","path":"/status/replicas","value":5}]`))
	require.NoError(t, cli.Patch(_ctx, _deploy, p))
	require.Equal(t, int64(5), _deploy.Object["spec"].(map[string]interface{})["replicas"])
	require.Equal(t, int64(5), _deploy.Object["status"].(map[string]interface{})["replicas"])
}
