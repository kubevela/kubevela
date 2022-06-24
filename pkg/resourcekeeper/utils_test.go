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

package resourcekeeper

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

var _ = Describe("Test ResourceKeeper utilities", func() {

	It("Test ClearNamespaceForClusterScopedResources", func() {
		cli := testClient
		app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"}}
		h := &resourceKeeper{
			Client:     cli,
			app:        app,
			applicator: apply.NewAPIApplicator(cli),
			cache:      newResourceCache(cli, app),
		}
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "vela"}}
		nsObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(ns)
		Expect(err).Should(Succeed())
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "vela"}}
		cmObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cm)
		Expect(err).Should(Succeed())
		uns := []*unstructured.Unstructured{{Object: nsObj}, {Object: cmObj}}
		uns[0].SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Namespace"))
		uns[1].SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
		h.ClearNamespaceForClusterScopedResources(uns)
		Expect(uns[0].GetNamespace()).Should(Equal(""))
		Expect(uns[1].GetNamespace()).Should(Equal("vela"))
	})

})
