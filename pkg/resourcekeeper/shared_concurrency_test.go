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

package resourcekeeper

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func sharedConfigMapPolicies() Policies {
	return Policies{SharedResource: &v1alpha1.SharedResourcePolicySpec{Rules: []v1alpha1.SharedResourcePolicyRule{{
		Selector: v1alpha1.ResourcePolicyRuleSelector{ResourceTypes: []string{"ConfigMap"}},
	}}}}
}

func renderedConfigMap(app, name string) *unstructured.Unstructured {
	cm := &unstructured.Unstructured{}
	cm.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
	cm.SetName(name)
	cm.SetNamespace("default")
	cm.SetLabels(map[string]string{oam.LabelAppName: app, oam.LabelAppNamespace: "default"})
	return cm
}

// bareConfigMap is what a workflow kube.#Delete step passes: only the identity of the resource.
func bareConfigMap(name string) *unstructured.Unstructured {
	cm := &unstructured.Unstructured{}
	cm.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
	cm.SetName(name)
	cm.SetNamespace("default")
	return cm
}

// An owner that joins the sharer list just before the last sharer deletes the resource must get it.
func TestResourceKeeperDeleteWhenASharerJoinsMeanwhile(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	joined := false
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if obj.GetName() == "target" && !joined {
				joined = true
				live := &v1.ConfigMap{}
				if err := c.Get(ctx, client.ObjectKeyFromObject(obj), live); err != nil {
					return err
				}
				live.Annotations[oam.AnnotationAppSharedBy] += ",default/late"
				if err := c.Update(ctx, live); err != nil {
					return err
				}
			}
			return c.Delete(ctx, obj, opts...)
		},
	}).Build()
	_rk, err := newAppKeeper(ctx, cli, &v1beta1.Application{
		ObjectMeta: v12.ObjectMeta{Name: "app", Namespace: "default", Generation: 1},
	}, Policies{})
	r.NoError(err)
	r.NoError(cli.Create(ctx, &v1.ConfigMap{ObjectMeta: v12.ObjectMeta{
		Name: "target", Namespace: "default",
		Labels:      map[string]string{oam.LabelAppName: "app", oam.LabelAppNamespace: "default"},
		Annotations: map[string]string{oam.AnnotationAppSharedBy: "default/app"},
	}}))

	r.NoError(_rk.Delete(ctx, []*unstructured.Unstructured{bareConfigMap("target")}))

	r.True(joined, "the test must have raced the delete")
	got := &v1.ConfigMap{}
	r.NoError(cli.Get(ctx, types.NamespacedName{Namespace: "default", Name: "target"}, got), "a resource another owner now shares must survive")
	r.Equal("default/late", got.Annotations[oam.AnnotationAppSharedBy])
	r.Equal("late", got.Labels[oam.LabelOwnerName], "and is handed to that owner")
}

// A sharer joins and the resource is then removed before the delete is retried: nothing is left to do.
func TestResourceKeeperDeleteWhenTheResourceGoesMeanwhile(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	gone := false
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if obj.GetName() != "target" || gone {
				return c.Delete(ctx, obj, opts...)
			}
			gone = true
			live := &v1.ConfigMap{}
			if err := c.Get(ctx, client.ObjectKeyFromObject(obj), live); err != nil {
				return err
			}
			if err := c.Delete(ctx, live); err != nil {
				return err
			}
			return kerrors.NewConflict(v1.Resource("configmaps"), "target", nil)
		},
	}).Build()
	_rk, err := newAppKeeper(ctx, cli, &v1beta1.Application{
		ObjectMeta: v12.ObjectMeta{Name: "app", Namespace: "default", Generation: 1},
	}, Policies{})
	r.NoError(err)
	r.NoError(cli.Create(ctx, &v1.ConfigMap{ObjectMeta: v12.ObjectMeta{
		Name: "target", Namespace: "default",
		Labels:      map[string]string{oam.LabelAppName: "app", oam.LabelAppNamespace: "default"},
		Annotations: map[string]string{oam.AnnotationAppSharedBy: "default/app"},
	}}))
	r.NoError(_rk.Delete(ctx, []*unstructured.Unstructured{bareConfigMap("target")}))
	r.True(gone)
}

// Several apps share and delete the same ConfigMaps at once while app1 keeps them: they must
// survive, shared by app1 alone, and go when app1 lets go.
func TestResourceKeeperConcurrentSharedResourceChurn(t *testing.T) {
	const apps, configMaps, rounds = 4, 5, 20
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()

	appName := func(i int) string { return fmt.Sprintf("app%d", i) }
	cmName := func(i int) string { return fmt.Sprintf("shared-%d", i) }
	keepers := map[string]ResourceKeeper{}
	for i := 1; i <= apps; i++ {
		rk, err := newAppKeeper(ctx, cli, &v1beta1.Application{
			ObjectMeta: v12.ObjectMeta{Name: appName(i), Namespace: "default", Generation: 1},
		}, sharedConfigMapPolicies())
		r.NoError(err)
		keepers[appName(i)] = rk
	}
	// a write that keeps losing races fails with a conflict, which a reconcile retries
	eventually := func(do func() error) error {
		var err error
		for i := 0; i < 100; i++ {
			if err = do(); err == nil {
				return nil
			}
		}
		return err
	}
	dispatch := func(app string) error {
		return eventually(func() error {
			var manifests []*unstructured.Unstructured
			for i := 0; i < configMaps; i++ {
				manifests = append(manifests, renderedConfigMap(app, cmName(i)))
			}
			return keepers[app].Dispatch(ctx, manifests, nil)
		})
	}
	remove := func(app string) error {
		return eventually(func() error {
			var manifests []*unstructured.Unstructured
			for i := 0; i < configMaps; i++ {
				manifests = append(manifests, bareConfigMap(cmName(i)))
			}
			return keepers[app].Delete(ctx, manifests)
		})
	}

	r.NoError(dispatch(appName(1)))
	var wg sync.WaitGroup
	errs := make(chan error, apps)
	for i := 2; i <= apps; i++ {
		wg.Add(1)
		go func(app string) {
			defer wg.Done()
			for n := 0; n < rounds; n++ {
				if err := dispatch(app); err != nil {
					errs <- fmt.Errorf("%s dispatch: %w", app, err)
					return
				}
				if err := remove(app); err != nil {
					errs <- fmt.Errorf("%s delete: %w", app, err)
					return
				}
			}
		}(appName(i))
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		r.NoError(err)
	}

	for i := 0; i < configMaps; i++ {
		got := &v1.ConfigMap{}
		r.NoError(cli.Get(ctx, types.NamespacedName{Namespace: "default", Name: cmName(i)}, got), "%s must survive while app1 has it", cmName(i))
		r.Equal("default/app1", got.Annotations[oam.AnnotationAppSharedBy], "%s must be shared by app1 alone", cmName(i))
		r.Equal("app1", got.Labels[oam.LabelAppName], "%s must be app1's", cmName(i))
	}

	r.NoError(remove(appName(1)))
	for i := 0; i < configMaps; i++ {
		err := cli.Get(ctx, types.NamespacedName{Namespace: "default", Name: cmName(i)}, &v1.ConfigMap{})
		r.True(kerrors.IsNotFound(err), "%s must go with its last sharer, got %v", cmName(i), err)
	}
}
