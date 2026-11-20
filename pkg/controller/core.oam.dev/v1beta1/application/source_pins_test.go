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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/sources"
)

func TestSensitivePinsAreKeptInASecretTheApplicationOwns(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "checkout", Namespace: "team-a", UID: "app-uid"}}
	vault := secretPins{client: cli, app: app}
	ctx := context.Background()

	require.Equal(t, "secret:workflow-checkout-sources/pins#db.password", vault.Ref("db.password"))
	got, err := vault.Load(ctx, "run-1")
	require.NoError(t, err)
	require.Nil(t, got, "nothing pinned yet")

	require.NoError(t, vault.Save(ctx, "run-1", map[string]interface{}{"db.password": "s3cret"}))
	secret := &corev1.Secret{}
	require.NoError(t, cli.Get(ctx, ktypes.NamespacedName{Namespace: "team-a", Name: "workflow-checkout-sources"}, secret))
	require.Len(t, secret.OwnerReferences, 1)
	require.Equal(t, ktypes.UID("app-uid"), secret.OwnerReferences[0].UID, "deleted with the Application")
	require.Equal(t, "checkout", secret.Labels["app.oam.dev/name"])

	got, err = vault.Load(ctx, "run-1")
	require.NoError(t, err)
	require.Equal(t, "s3cret", got["db.password"])
	got, err = vault.Load(ctx, "run-2")
	require.NoError(t, err)
	require.Nil(t, got, "another run's pins are not this one's")

	require.NoError(t, vault.Save(ctx, "run-2", map[string]interface{}{"db.password": "rotated"}))
	got, err = vault.Load(ctx, "run-2")
	require.NoError(t, err)
	require.Equal(t, "rotated", got["db.password"], "a new run replaces the last")
}

// A Secret of that name the Application does not control is neither read nor written: it may
// be a copy from a restore or another tenant's, and its contents are no pin of this run.
func TestPinsKeepToASecretTheApplicationControls(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "team-a", UID: "app-uid"}}
	earlier := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "team-a", UID: "earlier-uid"}}
	foreign := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: "workflow-shop-sources", Namespace: "team-a",
		OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(earlier, v1beta1.SchemeGroupVersion.WithKind(v1beta1.ApplicationKind))},
	}, Data: map[string][]byte{sensitivePinsKey: []byte(`{"run":"run-1","fields":{"db.password":"planted"}}`)}}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(foreign).Build()
	vault := secretPins{client: cli, app: app}

	got, err := vault.Load(context.Background(), "run-1")
	require.NoError(t, err)
	require.Nil(t, got, "a foreign Secret's pins are not this run's")

	err = vault.Save(context.Background(), "run-1", map[string]interface{}{"db.password": "s3cret"})
	require.ErrorContains(t, err, "is not controlled by Application team-a/shop")
	stored := &corev1.Secret{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(foreign), stored))
	require.Equal(t, foreign.Data, stored.Data, "left as it was")
}

func TestAnApplicationRevisionKeepsNoPinnedSources(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "workflow-checkout-context", Namespace: "team-a"},
		Data: map[string]string{"vars": `{"a":1}`, sources.PinnedSourcesKey: `{"run":"r","values":{"cfg":{"region":"eu-west"}}}`}}
	rev := &v1beta1.ApplicationRevision{ObjectMeta: metav1.ObjectMeta{Name: "checkout-v1", Namespace: "team-a"}}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm, rev).WithStatusSubresource(rev).Build()
	h := &AppHandler{Client: cli}
	h.UpdateApplicationRevisionStatus(context.Background(), rev, &common.WorkflowStatus{
		ContextBackend: &corev1.ObjectReference{Namespace: "team-a", Name: "workflow-checkout-context"}})
	require.Equal(t, map[string]string{"vars": `{"a":1}`}, rev.Status.WorkflowContext,
		"pins are for one run; the revision keeps the rest of the context")
	stored := &v1beta1.ApplicationRevision{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(rev), stored))
	require.NotContains(t, stored.Status.WorkflowContext, sources.PinnedSourcesKey)
}

// staleGets is a client whose cache has not seen any Secret yet.
type staleGets struct{ client.Client }

func (s staleGets) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*corev1.Secret); ok {
		return apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, key.Name)
	}
	return s.Client.Get(ctx, key, obj, opts...)
}

// Pins are read past the cache, which may not yet hold the Secret the last step created.
func TestPinsReadPastACacheThatIsBehind(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	live := fake.NewClientBuilder().WithScheme(scheme).Build()
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "checkout", Namespace: "team-a", UID: "app-uid"}}
	vault := secretPins{client: staleGets{live}, reader: live, app: app}
	ctx := context.Background()

	require.NoError(t, vault.Save(ctx, "run-1", map[string]interface{}{"db.password": "s3cret"}))
	require.NoError(t, vault.Save(ctx, "run-1", map[string]interface{}{"db.password": "rotated"}), "the second write updates, not creates")
	got, err := vault.Load(ctx, "run-1")
	require.NoError(t, err)
	require.Equal(t, "rotated", got["db.password"])
}

// A long Application name still gives a valid Secret name, the same one every time.
func TestPinsSecretNameFitsForALongApplicationName(t *testing.T) {
	long := strings.Repeat("a", 230) + "." + strings.Repeat("b", 22)
	vault := secretPins{app: &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: long, Namespace: "team-a"}}}
	name := vault.key().Name
	require.LessOrEqual(t, len(name), 253)
	require.Empty(t, validation.IsDNS1123Subdomain(name))
	require.Equal(t, name, vault.key().Name)
	require.Equal(t, "workflow-short-sources", secretPins{app: &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "short"}}}.key().Name)
}
