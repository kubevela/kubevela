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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
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

	It("keeps the sharer list consistent", func() {
		const apps, configMaps, rounds = 4, 5, 150
		ctx := context.Background()
		appName := func(i int) string { return fmt.Sprintf("churn-app%d", i) }
		cmName := func(i int) string { return fmt.Sprintf("churn-shared-%d", i) }
		keepers := map[string]ResourceKeeper{}
		for i := 1; i <= apps; i++ {
			rk, err := newAppKeeper(ctx, testClient, &v1beta1.Application{
				ObjectMeta: v12.ObjectMeta{Name: appName(i), Namespace: "default", Generation: 1},
			}, sharedConfigMapPolicies())
			Expect(err).Should(Succeed())
			keepers[appName(i)] = rk
		}
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

		Expect(dispatch(appName(1))).Should(Succeed())
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
			Expect(err).Should(Succeed())
		}

		for i := 0; i < configMaps; i++ {
			got := &v1.ConfigMap{}
			Expect(testClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: cmName(i)}, got)).Should(Succeed())
			Expect(got.Annotations[oam.AnnotationAppSharedBy]).Should(Equal("default/" + appName(1)))
			Expect(got.Labels[oam.LabelAppName]).Should(Equal(appName(1)))
		}

		Expect(remove(appName(1))).Should(Succeed())
		for i := 0; i < configMaps; i++ {
			err := testClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: cmName(i)}, &v1.ConfigMap{})
			Expect(kerrors.IsNotFound(err)).Should(BeTrue(), "%s: %v", cmName(i), err)
		}
	})
})
