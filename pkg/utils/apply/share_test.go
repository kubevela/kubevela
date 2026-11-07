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

package apply

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestShare(t *testing.T) {
	r := require.New(t)
	app := &v1beta1.Application{}
	app.SetName("app")
	app.SetNamespace("test")
	r.Equal("test/app", AddSharer("", appKey(app)))
	r.Equal("test/app,x/y", AddSharer("test/app,x/y", appKey(app)))
	r.Equal("x/y,test/app", AddSharer("x/y", appKey(app)))
	r.True(ContainsSharer("a/b,test/app,x/y", appKey(app)))
	r.False(ContainsSharer("a/b,x/y", appKey(app)))
	r.Equal("a/b", FirstSharer("a/b,x/y"))
	r.Equal("", FirstSharer(""))
	r.Equal("a/b,x/y", RemoveSharer("a/b,test/app,x/y", appKey(app)))
	r.Equal("a/b,x/y", RemoveSharer("a/b,x/y", appKey(app)))
}

// Owner keys have a canonical form, <kind>/<namespace>/<name>. Keys written before kinds
// existed (<namespace>/<name>, or <name> in the default namespace) belong to the configured
// default kind, and every sharer operation treats both forms of one owner as the same owner,
// so lists mixing them neither duplicate nor leak an entry.
func TestSharersCompareOwnerKeysCanonically(t *testing.T) {
	prev := CurrentConfig()
	defer Configure(prev)
	Configure(Config{DefaultOwnerKind: "Application"})
	r := require.New(t)

	r.Equal("Application/team-a/shop", CanonicalOwnerKey("team-a/shop"))
	r.Equal("Application/default/shop", CanonicalOwnerKey("shop"))
	r.Equal("Component/team-a/backend", CanonicalOwnerKey("Component/team-a/backend"))

	r.True(ContainsSharer("Application/team-a/shop", "team-a/shop"), "old form finds the canonical entry")
	r.True(ContainsSharer("team-a/shop", "Application/team-a/shop"), "canonical form finds the old entry")
	r.False(ContainsSharer("Component/team-a/shop", "team-a/shop"), "same name, different kind")

	r.Equal("Application/team-a/shop", AddSharer("Application/team-a/shop", "team-a/shop"), "no duplicate across forms")
	r.Equal("x/y,team-a/shop", AddSharer("x/y", "team-a/shop"), "a new sharer is written in the form given")

	r.Equal("Component/team-a/backend", RemoveSharer("team-a/shop,Application/team-a/shop,Component/team-a/backend", "team-a/shop"), "both forms of the leaver go")
	r.Equal("x/y", RemoveSharer("default/shop,x/y", "shop"), "a bare name is the default namespace")
}

// Without a default kind, kindless keys are left as they are and compared as strings.
func TestSharersWithoutADefaultKind(t *testing.T) {
	prev := CurrentConfig()
	defer Configure(prev)
	Configure(Config{})
	require.Equal(t, "team-a/shop", CanonicalOwnerKey("team-a/shop"))
	require.False(t, ContainsSharer("Application/team-a/shop", "team-a/shop"))
}

// A shared-by list can hold an empty entry: a legacy annotation, or one left by removing a
// sharer. Handing the resource to the next sharer must not pick it.
func TestFirstSharerSkipsEmptyEntries(t *testing.T) {
	require.Equal(t, "team-a/shop", FirstSharer(",team-a/shop"))
	require.Equal(t, "team-a/shop", FirstSharer("team-a/shop,team-b/other"))
	require.Empty(t, FirstSharer(""))
	require.Empty(t, FirstSharer(","))
	require.Equal(t, "team-b/other", RemoveSharer(",team-a/shop,team-b/other", "team-a/shop"),
		"removing a sharer drops the empty entries with it")
}

// An owner that left the list while a sharer was adding itself must not be written back.
func TestSharedApplyKeepsAConcurrentChangeToTheSharerList(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	raced := false
	cli := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			if !raced {
				raced = true
				live := &corev1.ConfigMap{}
				if err := c.Get(ctx, client.ObjectKeyFromObject(obj), live); err != nil {
					return err
				}
				if live.Annotations == nil {
					live.Annotations = map[string]string{}
				}
				live.Annotations[oam.AnnotationAppSharedBy] = "x/owner"
				if err := c.Update(ctx, live); err != nil {
					return err
				}
			}
			return c.Patch(ctx, obj, patch, opts...)
		},
	}).Build()
	r.NoError(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: "shared", Namespace: "default",
		Labels:      map[string]string{oam.LabelAppName: "owner", oam.LabelAppNamespace: "x"},
		Annotations: map[string]string{oam.AnnotationAppSharedBy: "x/owner,x/leaving"},
	}}))

	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"}}
	desired := &unstructured.Unstructured{}
	desired.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	desired.SetName("shared")
	desired.SetNamespace("default")
	r.NoError(NewAPIApplicator(cli).Apply(ctx, desired, sharedByApp(app)))

	r.True(raced, "the test must have raced the patch")
	got := &corev1.ConfigMap{}
	r.NoError(cli.Get(ctx, client.ObjectKey{Namespace: "default", Name: "shared"}, got))
	r.Equal("x/owner,default/app", got.Annotations[oam.AnnotationAppSharedBy])
}

// sharedConfigMapClient holds a ConfigMap controlled by owner, and runs race once before the
// first real patch.
func sharedConfigMapClient(t *testing.T, owner, sharedBy string, race func(c client.Client, live *corev1.ConfigMap) error) client.Client {
	raced := race == nil
	cli := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			if !raced {
				raced = true
				live := &corev1.ConfigMap{}
				if err := c.Get(ctx, client.ObjectKeyFromObject(obj), live); err != nil {
					return err
				}
				if err := race(c, live); err != nil {
					return err
				}
			}
			return c.Patch(ctx, obj, patch, opts...)
		},
	}).Build()
	require.NoError(t, cli.Create(context.Background(), &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: "shared", Namespace: "default",
		Labels:      map[string]string{oam.LabelAppName: owner, oam.LabelAppNamespace: "default"},
		Annotations: map[string]string{oam.AnnotationAppSharedBy: sharedBy},
	}}))
	return cli
}

func applyShared(cli client.Client, opts ...ApplyOption) error {
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"}}
	desired := &unstructured.Unstructured{}
	desired.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
	desired.SetName("shared")
	desired.SetNamespace("default")
	desired.SetLabels(map[string]string{oam.LabelAppName: "app", oam.LabelAppNamespace: "default"})
	return NewAPIApplicator(cli).Apply(context.Background(), desired, append(opts, sharedByApp(app))...)
}

func sharedByOf(t *testing.T, cli client.Client) string {
	got := &corev1.ConfigMap{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "shared"}, got))
	return got.Annotations[oam.AnnotationAppSharedBy]
}

func TestSharedApplyBySharer(t *testing.T) {
	t.Run("a dry run leaves the list alone", func(t *testing.T) {
		cli := sharedConfigMapClient(t, "owner", "default/owner", nil)
		require.NoError(t, applyShared(cli, DryRunAll()))
		require.Equal(t, "default/owner", sharedByOf(t, cli))
	})
	t.Run("fails if the resource stops being shared meanwhile", func(t *testing.T) {
		cli := sharedConfigMapClient(t, "owner", "default/owner", func(c client.Client, live *corev1.ConfigMap) error {
			delete(live.Annotations, oam.AnnotationAppSharedBy)
			return c.Update(context.Background(), live)
		})
		require.ErrorContains(t, applyShared(cli), "no longer shared")
	})
	t.Run("fails if the resource is deleted meanwhile", func(t *testing.T) {
		cli := sharedConfigMapClient(t, "owner", "default/owner", func(c client.Client, live *corev1.ConfigMap) error {
			return c.Delete(context.Background(), live)
		})
		require.True(t, kerrors.IsNotFound(errors.Cause(applyShared(cli))))
	})
}

func TestSharedApplyByOwnerMissingFromTheList(t *testing.T) {
	t.Run("the owner adds itself", func(t *testing.T) {
		cli := sharedConfigMapClient(t, "app", "x/other", nil)
		require.NoError(t, applyShared(cli))
		require.Equal(t, "x/other,default/app", sharedByOf(t, cli))
	})
	t.Run("a concurrent change to the list is a conflict", func(t *testing.T) {
		cli := sharedConfigMapClient(t, "app", "x/other", func(c client.Client, live *corev1.ConfigMap) error {
			live.Annotations[oam.AnnotationAppSharedBy] = "x/other,x/late"
			return c.Update(context.Background(), live)
		})
		require.True(t, kerrors.IsConflict(errors.Cause(applyShared(cli))))
		require.Equal(t, "x/other,x/late", sharedByOf(t, cli))
	})
}
