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

package application

import (
	"context"
	"errors"
	"testing"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// configMapCalls counts the client calls made on ConfigMaps.
type configMapCalls struct{ gets, creates, updates int }

func countingConfigMapClient(calls *configMapCalls, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(objs...).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.ConfigMap); ok {
				calls.gets++
			}
			return c.Get(ctx, key, obj, opts...)
		},
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*corev1.ConfigMap); ok {
				calls.creates++
			}
			return c.Create(ctx, obj, opts...)
		},
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if _, ok := obj.(*corev1.ConfigMap); ok {
				calls.updates++
			}
			return c.Update(ctx, obj, opts...)
		},
	}).Build()
}

func TestPolicyConfigMapNotReadForAppWithoutOne(t *testing.T) {
	calls := &configMapCalls{}
	h := &AppHandler{Client: countingConfigMapClient(calls)}
	app := appWithAnnotations(nil)
	record, err := h.previousPolicyMetadata(monitorContext.NewTraceContext(context.Background(), ""), app, nil)
	require.NoError(t, err)
	h.removeStoppedPolicyMetadata(app, record, nil)
	require.Zero(t, calls.gets, "an app no policy has applied to has no policy ConfigMap to read")
}

func TestPolicyConfigMapReadOncePerReconcile(t *testing.T) {
	app := appWithAnnotations(nil)
	app.Status.ApplicationPoliciesConfigMap = policyConfigMapName(app.Namespace, app.Name)
	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: app.Status.ApplicationPoliciesConfigMap, Namespace: app.Namespace},
		Data:       map[string]string{"metadata": `{"labels":{"team":"a"}}`},
	}
	calls := &configMapCalls{}
	h := &AppHandler{Client: countingConfigMapClient(calls, existing)}
	ctx := monitorContext.NewTraceContext(context.Background(), "")

	results := []RenderedPolicyResult{{PolicyName: "p", PolicyType: "p", Enabled: true, Transforms: &PolicyOutput{Labels: map[string]string{"team": "b"}}}}
	record, err := h.previousPolicyMetadata(ctx, app, results)
	require.NoError(t, err)
	h.removeStoppedPolicyMetadata(app, record, results)
	h.writePolicyObservabilityConfigMap(ctx, app, results, &v1beta1.ApplicationSpec{}, extractRenderedMetadata(results), false, false)

	require.Equal(t, 1, calls.gets, "the ConfigMap read for the metadata record is the one the write updates")
	require.Zero(t, calls.creates, "a ConfigMap already read is updated, not created")
	require.Equal(t, 1, calls.updates)

	stored := &corev1.ConfigMap{}
	require.NoError(t, h.Client.Get(context.Background(), client.ObjectKeyFromObject(existing), stored))
	require.Contains(t, stored.Data["metadata"], `"team": "b"`)
}

// recordedConfigMap is app's policy ConfigMap recording that the policies set label team=a.
func recordedConfigMap(app *v1beta1.Application, uid string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: policyConfigMapName(app.Namespace, app.Name), Namespace: app.Namespace,
			Labels: map[string]string{oam.LabelAppUID: uid}},
		Data: map[string]string{"metadata": `{"labels":{"team":"a"}}`},
	}
}

func TestPolicyConfigMapReadErrorIsReturned(t *testing.T) {
	app := appWithAnnotations(nil)
	app.UID = "uid-1"
	app.Status.ApplicationPoliciesConfigMap = policyConfigMapName(app.Namespace, app.Name)
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(recordedConfigMap(app, "uid-1")).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if _, ok := obj.(*corev1.ConfigMap); ok {
				return errors.New("etcd timeout")
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}).Build()
	h := &AppHandler{Client: cli}
	_, err := h.previousPolicyMetadata(monitorContext.NewTraceContext(context.Background(), ""), app, nil)
	require.ErrorContains(t, err, "etcd timeout", "an unreadable record is not an empty one: the rewrite would lose the keys to remove")
}

func TestPolicyConfigMapReadWithoutStatusWhenPoliciesRender(t *testing.T) {
	app := appWithAnnotations(nil)
	app.UID = "uid-1"
	h := &AppHandler{Client: fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(recordedConfigMap(app, "uid-1")).Build()}
	results := []RenderedPolicyResult{{PolicyName: "p", PolicyType: "p", Enabled: true}}
	record, err := h.previousPolicyMetadata(monitorContext.NewTraceContext(context.Background(), ""), app, results)
	require.NoError(t, err)
	require.Equal(t, "a", record.labels["team"].value, "a ConfigMap whose name the status failed to record is still the record")
}

func TestPolicyConfigMapOfAnotherAppIsNoRecord(t *testing.T) {
	for name, uid := range map[string]string{
		"left by a deleted Application of the same name": "uid-1",
		"carrying no Application uid":                    "",
	} {
		t.Run(name, func(t *testing.T) {
			app := appWithAnnotations(nil)
			app.UID = "uid-2"
			cm := recordedConfigMap(app, uid)
			if uid == "" {
				cm.Labels = nil
			}
			h := &AppHandler{Client: fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(cm).Build()}
			results := []RenderedPolicyResult{{PolicyName: "p", PolicyType: "p", Enabled: true}}
			record, err := h.previousPolicyMetadata(monitorContext.NewTraceContext(context.Background(), ""), app, results)
			require.NoError(t, err)
			require.Empty(t, record.labels, "only a ConfigMap labelled with this Application's uid is its record")
			require.NotNil(t, h.policyConfigMap, "it is still the object the write replaces")
		})
	}
}
