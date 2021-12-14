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

package resourcekeeper

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	v13 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

var _ = Describe("Test ResourceKeeper StateKeep", func() {
	It("Test StateKeep for various scene", func() {
		cli := testClient
		createConfigMap := func(name string, value string) *unstructured.Unstructured {
			o := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      name,
						"namespace": "default",
					},
					"data": map[string]interface{}{
						"key": value,
					},
				},
			}
			o.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
			return o
		}

		// state-keep add this resource
		cm1 := createConfigMap("cm1", "value")
		cmRaw1, err := json.Marshal(cm1)
		Expect(err).Should(Succeed())

		// state-keep skip this resource
		cm2 := createConfigMap("cm2", "value")
		Expect(cli.Create(context.Background(), cm2)).Should(Succeed())

		// state-keep delete this resource
		cm3 := createConfigMap("cm3", "value")
		Expect(cli.Create(context.Background(), cm3)).Should(Succeed())

		// state-keep delete this resource
		cm4 := createConfigMap("cm4", "value")
		cmRaw4, err := json.Marshal(cm4)
		Expect(err).Should(Succeed())
		Expect(cli.Create(context.Background(), cm4)).Should(Succeed())

		// state-keep update this resource
		cm5 := createConfigMap("cm5", "value")
		cmRaw5, err := json.Marshal(cm5)
		Expect(err).Should(Succeed())
		cm5.Object["data"].(map[string]interface{})["key"] = "changed"
		Expect(cli.Create(context.Background(), cm5)).Should(Succeed())

		createConfigMapClusterObjectReference := func(name string) common2.ClusterObjectReference {
			return common2.ClusterObjectReference{
				ObjectReference: v1.ObjectReference{
					Kind:       "ConfigMap",
					APIVersion: v1.SchemeGroupVersion.String(),
					Name:       name,
					Namespace:  "default",
				},
			}
		}

		h := &resourceKeeper{
			Client:     cli,
			app:        &v1beta1.Application{ObjectMeta: v13.ObjectMeta{Name: "app", Namespace: "default"}},
			applicator: apply.NewAPIApplicator(cli),
			cache:      newResourceCache(cli),
		}

		h._currentRT = &v1beta1.ResourceTracker{
			Spec: v1beta1.ResourceTrackerSpec{
				ManagedResources: []v1beta1.ManagedResource{{
					ClusterObjectReference: createConfigMapClusterObjectReference("cm1"),
					Data:                   &runtime.RawExtension{Raw: cmRaw1},
				}, {
					ClusterObjectReference: createConfigMapClusterObjectReference("cm2"),
				}, {
					ClusterObjectReference: createConfigMapClusterObjectReference("cm3"),
					Deleted:                true,
				}, {
					ClusterObjectReference: createConfigMapClusterObjectReference("cm4"),
					Data:                   &runtime.RawExtension{Raw: cmRaw4},
					Deleted:                true,
				}, {
					ClusterObjectReference: createConfigMapClusterObjectReference("cm5"),
					Data:                   &runtime.RawExtension{Raw: cmRaw5},
				}},
			},
		}

		Expect(h.StateKeep(context.Background())).Should(Succeed())
		cms := &unstructured.UnstructuredList{}
		cms.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ConfigMap"))
		Expect(cli.List(context.Background(), cms, client.InNamespace("default"))).Should(Succeed())
		Expect(len(cms.Items)).Should(Equal(3))
		Expect(cms.Items[0].GetName()).Should(Equal("cm1"))
		Expect(cms.Items[1].GetName()).Should(Equal("cm2"))
		Expect(cms.Items[2].GetName()).Should(Equal("cm5"))
		Expect(cms.Items[2].Object["data"].(map[string]interface{})["key"].(string)).Should(Equal("value"))

		Expect(cli.Get(context.Background(), client.ObjectKeyFromObject(cm1), cm1)).Should(Succeed())
		cm1.SetLabels(map[string]string{
			oam.LabelAppName:      "app-2",
			oam.LabelAppNamespace: "default",
		})
		Expect(cli.Update(context.Background(), cm1)).Should(Succeed())
		err = h.StateKeep(context.Background())
		Expect(err).ShouldNot(Succeed())
		Expect(err.Error()).Should(ContainSubstring("failed to re-apply"))
	})
})
