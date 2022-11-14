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

package utils

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/utils/errors"
)

var _ = Describe("Test Create Or Update Namespace functions", func() {

	BeforeEach(func() {
	})

	It("Test Create namespace function", func() {

		By("test a normal namespace create case that should be created")
		namespaceName := "my-test-test1"
		err := CreateNamespace(context.Background(), k8sClient, namespaceName)
		Expect(err).Should(BeNil())
		var gotNS v1.Namespace
		err = k8sClient.Get(context.Background(), client.ObjectKey{Name: namespaceName}, &gotNS)
		Expect(err).Should(BeNil())

		By("test a namespace create with no annotations case that should be created")
		namespaceName = "my-test-test2"
		var overrideAnn map[string]string
		var overrideLabels map[string]string
		err = CreateNamespace(context.Background(), k8sClient, namespaceName, MergeOverrideAnnotations(overrideAnn), MergeOverrideLabels(overrideLabels))
		Expect(err).Should(BeNil())
		err = k8sClient.Get(context.Background(), client.ObjectKey{Name: namespaceName}, &gotNS)
		Expect(err).Should(BeNil())
		Expect(gotNS.Annotations).Should(BeNil())

		By("test a namespace create with annotations case that should be created")
		namespaceName = "my-test-test3"
		overrideAnn = map[string]string{"abc": "xyz", "haha": "123"}
		overrideLabels = map[string]string{"l1": "v1", "l2": "v2"}
		err = CreateNamespace(context.Background(), k8sClient, namespaceName, MergeOverrideAnnotations(overrideAnn), MergeOverrideLabels(overrideLabels))
		Expect(err).Should(BeNil())
		err = k8sClient.Get(context.Background(), client.ObjectKey{Name: namespaceName}, &gotNS)
		Expect(err).Should(BeNil())
		Expect(gotNS.Annotations).Should(BeEquivalentTo(overrideAnn))
		for k, v := range overrideLabels {
			Expect(gotNS.Labels).Should(HaveKeyWithValue(k, v))
		}

	})

	It("Test Update namespace function", func() {

		By("test a normal namespace update with no not found error")
		namespaceName := "updatetest-test1"
		err := UpdateNamespace(context.Background(), k8sClient, namespaceName)
		Expect(apierror.IsNotFound(err)).Should(BeTrue())

		overrideAnn := map[string]string{"abc": "xyz", "haha": "123"}
		overrideLabels := map[string]string{"l1": "v1", "l2": "v2"}
		err = CreateNamespace(context.Background(), k8sClient, namespaceName, MergeOverrideAnnotations(overrideAnn), MergeOverrideLabels(overrideLabels))
		Expect(err).Should(BeNil())

		By("test a namespace update with merge labels and annotations case that should be updated")
		overrideAnn = map[string]string{"haha": "456", "newkey": "newvalue"}
		overrideLabels = map[string]string{"l2": "v4", "l3": "v3"}
		err = UpdateNamespace(context.Background(), k8sClient, namespaceName, MergeOverrideAnnotations(overrideAnn), MergeOverrideLabels(overrideLabels))
		Expect(err).Should(BeNil())
		var gotNS v1.Namespace
		err = k8sClient.Get(context.Background(), client.ObjectKey{Name: namespaceName}, &gotNS)
		Expect(err).Should(BeNil())
		Expect(gotNS.Annotations).Should(BeEquivalentTo(map[string]string{"abc": "xyz", "haha": "456", "newkey": "newvalue"}))
		for k, v := range overrideLabels {
			Expect(gotNS.Labels).Should(HaveKeyWithValue(k, v))
		}

		By("test a namespace update with no conflict label key-value case that should be updated")
		overrideLabels = map[string]string{"l2": "v5"}
		noconflictLabels := map[string]string{"nc1": "v5"}
		err = UpdateNamespace(context.Background(), k8sClient, namespaceName, MergeNoConflictLabels(noconflictLabels), MergeOverrideLabels(overrideLabels))
		Expect(err).Should(BeNil())
		err = k8sClient.Get(context.Background(), client.ObjectKey{Name: namespaceName}, &gotNS)
		Expect(err).Should(BeNil())
		for k, v := range noconflictLabels {
			Expect(gotNS.Labels).Should(HaveKeyWithValue(k, v))
		}
		for k, v := range overrideLabels {
			Expect(gotNS.Labels).Should(HaveKeyWithValue(k, v))
		}

		By("test a namespace update with conflict label that should return error")
		noconflictLabels = map[string]string{"l2": "v6"}
		err = UpdateNamespace(context.Background(), k8sClient, namespaceName, MergeNoConflictLabels(noconflictLabels))
		Expect(err).ShouldNot(BeNil())
		Expect(errors.IsLabelConflict(err)).Should(BeTrue())

		By("test a namespace update with conflict key but same key should not return error")
		noconflictLabels = map[string]string{"l2": "v5"}
		err = UpdateNamespace(context.Background(), k8sClient, namespaceName, MergeNoConflictLabels(noconflictLabels))
		Expect(err).Should(BeNil())

		By("test a namespace update with reset key to be empty")
		overrideLabels = map[string]string{"l1": "", "l2": ""}
		err = UpdateNamespace(context.Background(), k8sClient, namespaceName, MergeOverrideLabels(overrideLabels))
		Expect(err).Should(BeNil())
		err = k8sClient.Get(context.Background(), client.ObjectKey{Name: namespaceName}, &gotNS)
		Expect(err).Should(BeNil())
		for k, v := range overrideLabels {
			Expect(gotNS.Labels).Should(HaveKeyWithValue(k, v))
		}

		By("test a namespace update with conflict label key but the exist value is empty should be able to change")
		noconflictLabels = map[string]string{"l1": "vx", "l2": "vy"}
		err = UpdateNamespace(context.Background(), k8sClient, namespaceName, MergeNoConflictLabels(noconflictLabels))
		Expect(err).Should(BeNil())
		err = k8sClient.Get(context.Background(), client.ObjectKey{Name: namespaceName}, &gotNS)
		Expect(err).Should(BeNil())
		for k, v := range noconflictLabels {
			Expect(gotNS.Labels).Should(HaveKeyWithValue(k, v))
		}
	})

	It("Test CreateOrUpdate namespace function", func() {
		By("test a normal namespace update with no namespace exist")
		namespaceName := "create-or-update-test1"
		err := CreateOrUpdateNamespace(context.Background(), k8sClient, namespaceName)
		Expect(err).Should(BeNil())

		By("test update namespace with functions")
		overrideAnn := map[string]string{"abc": "xyz", "haha": "123"}
		overrideLabels := map[string]string{"l1": "v1", "l2": "v2"}
		noconflictLabels := map[string]string{"c1": "v1", "c2": "v2"}
		err = CreateOrUpdateNamespace(context.Background(), k8sClient, namespaceName, MergeOverrideAnnotations(overrideAnn), MergeOverrideLabels(overrideLabels), MergeNoConflictLabels(noconflictLabels))
		Expect(err).Should(BeNil())
		var gotNS v1.Namespace
		err = k8sClient.Get(context.Background(), client.ObjectKey{Name: namespaceName}, &gotNS)
		Expect(err).Should(BeNil())
		Expect(gotNS.Annotations).Should(BeEquivalentTo(overrideAnn))
		for k, v := range overrideLabels {
			Expect(gotNS.Labels).Should(HaveKeyWithValue(k, v))
		}
		for k, v := range noconflictLabels {
			Expect(gotNS.Labels).Should(HaveKeyWithValue(k, v))
		}
	})

	It("Test IsClusterScope", func() {
		ok, err := IsClusterScope(v1.SchemeGroupVersion.WithKind("ConfigMap"), k8sClient.RESTMapper())
		Expect(err).Should(Succeed())
		Expect(ok).Should(BeFalse())
		ok, err = IsClusterScope(rbacv1.SchemeGroupVersion.WithKind("ClusterRole"), k8sClient.RESTMapper())
		Expect(err).Should(Succeed())
		Expect(ok).Should(BeTrue())
	})

	It("Test FilterObjectsByFieldSelector function", func() {
		var componentSpec = v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "test1-component",
					Type:       "worker",
					Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
				},
			},
		}
		var componentStatus = common.AppStatus{
			Services: []common.ApplicationComponentStatus{
				{
					Name:    "test1-component",
					Message: "test1-component applied",
					Healthy: true,
				},
			},
			Phase: common.ApplicationRunning,
		}
		apps := []*v1beta1.Application{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app1",
					Namespace: "test2",
				},
				Spec:   componentSpec,
				Status: componentStatus,
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app2",
					Namespace: "test2",
				},
				Spec:   componentSpec,
				Status: componentStatus,
			},
		}

		var objects []runtime.Object
		for i := range apps {
			objects = append(objects, apps[i])
		}
		fieldSelector, err := fields.ParseSelector("metadata.name=app2,metadata.namespace=test2")
		Expect(err).Should(BeNil())
		filteredObjects := FilterObjectsByFieldSelector(objects, fieldSelector)
		Expect(filteredObjects).Should(ContainElements(objects[1]))
	})
})
