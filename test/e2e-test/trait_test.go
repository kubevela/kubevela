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

package controllers_test

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func readAppFromFile(filename string) (*v1beta1.Application, error) {
	bs, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	app := &v1beta1.Application{}
	if err = yaml.Unmarshal(bs, app); err != nil {
		return nil, err
	}
	return app, nil
}

var _ = Describe("Trait tests", func() {
	ctx := context.Background()
	var namespace string

	BeforeEach(func() {
		namespace = randomNamespaceName("trait-test")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: namespace}})).Should(Succeed())
	})

	AfterEach(func() {
		ns := &corev1.Namespace{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: namespace}, ns)).Should(Succeed())
		Expect(k8sClient.Delete(ctx, ns)).Should(Succeed())
	})

	Context("Test app with traits", func() {
		It("Test json-patch trait", func() {
			app, err := readAppFromFile("../../docs/examples/traits/json-patch/example.yaml")
			Expect(err).Should(Succeed())
			app.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				deploy := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "busybox"}, deploy)).Should(Succeed())
				g.Expect(deploy.Labels).ShouldNot(BeNil())
				g.Expect(deploy.Spec.Replicas).Should(Equal(pointer.Int32(3)))
				g.Expect(deploy.Spec.Template.ObjectMeta.Labels).ShouldNot(BeNil())
				g.Expect(deploy.Spec.Template.ObjectMeta.Labels["pod-label-key"]).Should(Equal("pod-label-modified-value"))
				g.Expect(deploy.Spec.Template.ObjectMeta.Labels["to-delete-label-key"]).ShouldNot(Equal("to-delete-label-value"))
				g.Expect(len(deploy.Spec.Template.Spec.Containers)).Should(Equal(2))
				g.Expect(deploy.Spec.Template.Spec.Containers[1].Name).Should(Equal("busybox-sidecar"))
				g.Expect(deploy.Spec.Template.Spec.Containers[1].Image).Should(Equal("busybox:1.34"))
				g.Expect(deploy.Spec.Template.Spec.Containers[1].Command).Should(Equal([]string{"sleep", "864000"}))
			}, 15*time.Second).Should(Succeed())
		})

		It("Test json-merge-patch trait", func() {
			app, err := readAppFromFile("../../docs/examples/traits/json-merge-patch/example.yaml")
			Expect(err).Should(Succeed())
			app.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			Eventually(func(g Gomega) {
				deploy := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "busybox"}, deploy)).Should(Succeed())
				g.Expect(deploy.Labels).ShouldNot(BeNil())
				g.Expect(deploy.Labels["deploy-label-key"]).Should(Equal("deploy-label-added-value"))
				g.Expect(deploy.Spec.Replicas).Should(Equal(pointer.Int32(3)))
				g.Expect(deploy.Spec.Template.ObjectMeta.Labels).ShouldNot(BeNil())
				g.Expect(deploy.Spec.Template.ObjectMeta.Labels["pod-label-key"]).Should(Equal("pod-label-modified-value"))
				_, exists := deploy.Spec.Template.ObjectMeta.Labels["to-delete-label-key"]
				g.Expect(exists).Should(BeFalse())
				g.Expect(len(deploy.Spec.Template.Spec.Containers)).Should(Equal(1))
				g.Expect(deploy.Spec.Template.Spec.Containers[0].Name).Should(Equal("busybox-new"))
				g.Expect(deploy.Spec.Template.Spec.Containers[0].Image).Should(Equal("busybox:1.34"))
				g.Expect(deploy.Spec.Template.Spec.Containers[0].Command).Should(Equal([]string{"sleep", "864000"}))
			}, 15*time.Second).Should(Succeed())
		})
	})
})
