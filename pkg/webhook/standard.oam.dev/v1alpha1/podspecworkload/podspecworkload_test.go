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

package podspecworkload

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/common"
)

func TestPodSpecWorkload(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PodSpecWorkload Suite")
}

var _ = Describe("Test PodSpecWorkload", func() {
	var baseCase v1alpha1.PodSpecWorkload
	ctx := common.NewReconcileContext(context.Background(), types.NamespacedName{})

	BeforeEach(func() {
		baseCase = v1alpha1.PodSpecWorkload{
			ObjectMeta: metav1.ObjectMeta{
				Name: "mutate-hook",
			},
			Spec: v1alpha1.PodSpecWorkloadSpec{},
		}
	})

	It("Test with fill in all default", func() {
		cw := baseCase
		want := baseCase
		want.Spec.Replicas = pointer.Int32Ptr(1)
		DefaultPodSpecWorkload(ctx, &cw)
		Expect(cw).Should(BeEquivalentTo(want))
	})

	It("Test only fill in empty fields", func() {
		cw := baseCase
		cw.Spec.Replicas = pointer.Int32Ptr(10)
		want := cw
		DefaultPodSpecWorkload(ctx, &cw)
		Expect(cw).Should(BeEquivalentTo(want))
	})

	It("Test validate valid trait", func() {
		cw := baseCase
		cw.ObjectMeta.Namespace = "default"
		cw.Spec.Replicas = pointer.Int32Ptr(5)
		cw.Spec.PodSpec.Containers = []v1.Container{
			{
				Name:  "test",
				Image: "test",
			},
		}
		Expect(ValidateCreate(ctx, &cw).ToAggregate()).NotTo(HaveOccurred())
		Expect(ValidateUpdate(ctx, &cw, nil).ToAggregate()).NotTo(HaveOccurred())
		Expect(ValidateDelete(ctx, &cw).ToAggregate()).NotTo(HaveOccurred())
	})

	It("Test validate invalid trait", func() {
		cw := baseCase
		cw.Spec.Replicas = pointer.Int32Ptr(-5)
		Expect(ValidateCreate(ctx, &cw).ToAggregate()).To(HaveOccurred())
		Expect(ValidateUpdate(ctx, &cw, nil).ToAggregate()).To(HaveOccurred())
		Expect(len(ValidateCreate(ctx, &cw))).Should(Equal(3))
		// add namespace
		cw.ObjectMeta.Namespace = "default"
		Expect(len(ValidateCreate(ctx, &cw))).Should(Equal(2))
		// get valid replica
		cw.Spec.Replicas = pointer.Int32Ptr(5)
		Expect(len(ValidateCreate(ctx, &cw))).Should(Equal(1))
	})
})
