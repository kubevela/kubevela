/*
Copyright 2026 The KubeVela Authors.

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

package features

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestPublish(t *testing.T) {
	ctx := context.Background()
	gate := featuregate.NewFeatureGate()
	require.NoError(t, gate.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		"On":          {Default: true},
		"Off":         {Default: false},
		"NotKubeVela": {Default: true},
	}))
	names := []featuregate.Feature{"Off", "On"}
	cli := fake.NewClientBuilder().Build()
	read := func() *corev1.ConfigMap {
		cm := &corev1.ConfigMap{}
		require.NoError(t, cli.Get(ctx, types.NamespacedName{Namespace: "vela-system", Name: FeatureGatesConfigMapName}, cm))
		return cm
	}

	require.NoError(t, Publish(ctx, cli, "vela-system", "v1.12.0", gate, names))
	cm := read()
	assert.Equal(t, map[string]string{"On": "true", "Off": "false"}, cm.Data, "the named gates, not the others sharing the feature gate")
	assert.Equal(t, "v1.12.0", cm.Annotations[FeatureGatesVersionAnnotation])
	assert.Equal(t, "kubevela", cm.Labels["app.kubernetes.io/part-of"], "created as KubeVela's own")

	require.NoError(t, gate.Set("Off=true"))
	require.NoError(t, Publish(ctx, cli, "vela-system", "v1.12.1", gate, names))
	cm = read()
	assert.Equal(t, "true", cm.Data["Off"], "a restart with other flags is written over the last")
	assert.Equal(t, "v1.12.1", cm.Annotations[FeatureGatesVersionAnnotation])
}

func TestPublishRefusesAConfigMapKubeVelaDoesNotOwn(t *testing.T) {
	ctx := context.Background()
	gate := featuregate.NewFeatureGate()
	require.NoError(t, gate.Add(map[featuregate.Feature]featuregate.FeatureSpec{"On": {Default: true}}))
	theirs := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: FeatureGatesConfigMapName, Namespace: "vela-system"},
		Data:       map[string]string{"theirs": "data"},
	}
	cli := fake.NewClientBuilder().WithObjects(theirs).Build()

	err := Publish(ctx, cli, "vela-system", "v1.12.0", gate, []featuregate.Feature{"On"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vela-system/"+FeatureGatesConfigMapName)
	assert.Contains(t, err.Error(), "app.kubernetes.io/part-of=kubevela")

	cm := &corev1.ConfigMap{}
	require.NoError(t, cli.Get(ctx, types.NamespacedName{Namespace: "vela-system", Name: FeatureGatesConfigMapName}, cm))
	assert.Equal(t, map[string]string{"theirs": "data"}, cm.Data, "left as it was")
}

func TestPublishRetriesOnConflict(t *testing.T) {
	ctx := context.Background()
	gate := featuregate.NewFeatureGate()
	require.NoError(t, gate.Add(map[featuregate.Feature]featuregate.FeatureSpec{"On": {Default: true}}))
	ours := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      FeatureGatesConfigMapName,
			Namespace: "vela-system",
			Labels:    map[string]string{"app.kubernetes.io/part-of": "kubevela"},
		},
		Data: map[string]string{"On": "false"},
	}
	updates := 0
	cli := fake.NewClientBuilder().WithObjects(ours).WithInterceptorFuncs(interceptor.Funcs{
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			updates++
			if updates == 1 {
				return apierrors.NewConflict(schema.GroupResource{Resource: "configmaps"}, obj.GetName(), nil)
			}
			return c.Update(ctx, obj, opts...)
		},
	}).Build()

	require.NoError(t, Publish(ctx, cli, "vela-system", "v1.12.0", gate, []featuregate.Feature{"On"}))
	assert.Equal(t, 2, updates, "a conflict is retried on a fresh read")
	cm := &corev1.ConfigMap{}
	require.NoError(t, cli.Get(ctx, types.NamespacedName{Namespace: "vela-system", Name: FeatureGatesConfigMapName}, cm))
	assert.Equal(t, "true", cm.Data["On"])
}

func TestKubeVelaFeatures(t *testing.T) {
	names := KubeVelaFeatures()
	assert.Contains(t, names, EnableDefinitionInheritance)
	assert.NotContains(t, names, featuregate.Feature("APIListChunking"), "an API server gate in the same process")
}
