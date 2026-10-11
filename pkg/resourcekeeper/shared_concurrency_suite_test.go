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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Test shared resources under concurrent share and delete", func() {
	// The fix relies on the API server refusing these two writes once the object has changed.
	It("the API server rejects a stale conditional patch and delete", func() {
		ctx := context.Background()
		cm := &v1.ConfigMap{ObjectMeta: v12.ObjectMeta{Name: "churn-stale", Namespace: "default"}}
		Expect(testClient.Create(ctx, cm)).Should(Succeed())
		stale := cm.DeepCopy()
		cm.Data = map[string]string{"changed": "yes"}
		Expect(testClient.Update(ctx, cm)).Should(Succeed())

		patch := []byte(fmt.Sprintf(`{"metadata":{"resourceVersion":%q,"annotations":{"a":"b"}}}`, stale.ResourceVersion))
		err := testClient.Patch(ctx, stale.DeepCopy(), client.RawPatch(types.MergePatchType, patch))
		Expect(kerrors.IsConflict(err)).Should(BeTrue(), "patch: %v", err)

		uid, rv := stale.UID, stale.ResourceVersion
		err = testClient.Delete(ctx, stale.DeepCopy(), client.Preconditions{UID: &uid, ResourceVersion: &rv})
		Expect(kerrors.IsConflict(err)).Should(BeTrue(), "delete: %v", err)
		Expect(testClient.Delete(ctx, cm)).Should(Succeed())
	})

	// racingClient changes the sharer list of the named ConfigMap just before the first write of
	// the given kind reaches the API server.
	racingClient := func(name, sharedBy string, onDelete bool) client.Client {
		c, err := client.NewWithWatch(testEnv.Config, client.Options{Scheme: common.Scheme})
		Expect(err).Should(Succeed())
		raced := false
		race := func(ctx context.Context, obj client.Object) error {
			if obj.GetName() != name || raced {
				return nil
			}
			raced = true
			live := &v1.ConfigMap{}
			if err := c.Get(ctx, client.ObjectKeyFromObject(obj), live); err != nil {
				return err
			}
			live.Annotations[oam.AnnotationAppSharedBy] = sharedBy
			return c.Update(ctx, live)
		}
		return interceptor.NewClient(c, interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if onDelete {
					if err := race(ctx, obj); err != nil {
						return err
					}
				}
				return c.Delete(ctx, obj, opts...)
			},
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				// the pre-dispatch dry run patches too: race the real write
				if !onDelete && len((&client.PatchOptions{}).ApplyOptions(opts).DryRun) == 0 {
					if err := race(ctx, obj); err != nil {
						return err
					}
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		})
	}
	sharedConfigMap := func(name, owner, sharedBy string) *v1.ConfigMap {
		return &v1.ConfigMap{ObjectMeta: v12.ObjectMeta{
			Name: name, Namespace: "default",
			Labels:      map[string]string{oam.LabelAppName: owner, oam.LabelAppNamespace: "default"},
			Annotations: map[string]string{oam.AnnotationAppSharedBy: sharedBy},
		}}
	}

	It("hands the resource to a sharer that joins just before the last sharer deletes it", func() {
		ctx := context.Background()
		Expect(testClient.Create(ctx, sharedConfigMap("race-join", "race-join-app", "default/race-join-app"))).Should(Succeed())
		rk, err := newAppKeeper(ctx, racingClient("race-join", "default/race-join-app,default/race-late", true), &v1beta1.Application{
			ObjectMeta: v12.ObjectMeta{Name: "race-join-app", Namespace: "default", Generation: 1},
		}, Policies{})
		Expect(err).Should(Succeed())
		Expect(rk.Delete(ctx, []*unstructured.Unstructured{bareConfigMap("race-join")})).Should(Succeed())

		got := &v1.ConfigMap{}
		Expect(testClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "race-join"}, got)).Should(Succeed())
		Expect(got.Annotations[oam.AnnotationAppSharedBy]).Should(Equal("default/race-late"))
		Expect(got.Labels[oam.LabelOwnerName]).Should(Equal("race-late"))
		Expect(testClient.Delete(ctx, got)).Should(Succeed())
	})

	It("does not write back a sharer that leaves while another joins", func() {
		ctx := context.Background()
		Expect(testClient.Create(ctx, sharedConfigMap("race-leave", "race-owner", "default/race-owner,default/race-leaving"))).Should(Succeed())
		rk, err := newAppKeeper(ctx, racingClient("race-leave", "default/race-owner", false), &v1beta1.Application{
			ObjectMeta: v12.ObjectMeta{Name: "race-leave-app", Namespace: "default", Generation: 1},
		}, sharedConfigMapPolicies())
		Expect(err).Should(Succeed())
		Expect(rk.Dispatch(ctx, []*unstructured.Unstructured{renderedConfigMap("race-leave-app", "race-leave")}, nil)).Should(Succeed())

		got := &v1.ConfigMap{}
		Expect(testClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "race-leave"}, got)).Should(Succeed())
		Expect(got.Annotations[oam.AnnotationAppSharedBy]).Should(Equal("default/race-owner,default/race-leave-app"))
		Expect(testClient.Delete(ctx, got)).Should(Succeed())
	})
})
