package containerized_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/cloud-native-application/rudrx/api/v1alpha1"
	. "github.com/cloud-native-application/rudrx/pkg/webhook/containerized"
)

var _ = Describe("Containerized", func() {
	var baseCase v1alpha1.Containerized

	BeforeEach(func() {
		baseCase = v1alpha1.Containerized{
			ObjectMeta: metav1.ObjectMeta{
				Name: "mutate-hook",
			},
			Spec: v1alpha1.ContainerizedSpec{},
		}
	})

	It("Test with fill in all default", func() {
		cw := baseCase
		want := baseCase
		want.Spec.Replicas = pointer.Int32Ptr(1)
		Default(&cw)
		Expect(cw).Should(BeEquivalentTo(want))
	})

	It("Test only fill in empty fields", func() {
		cw := baseCase
		cw.Spec.Replicas = pointer.Int32Ptr(10)
		want := cw
		Default(&cw)
		Expect(cw).Should(BeEquivalentTo(want))
	})

	It("Test validate valid trait", func() {
		cw := baseCase
		cw.Spec.Replicas = pointer.Int32Ptr(5)
		cw.Spec.PodSpec.Containers = []v1.Container{
			{
				Name:  "test",
				Image: "test",
			},
		}
		Expect(ValidateCreate(&cw).ToAggregate()).NotTo(HaveOccurred())
		Expect(ValidateUpdate(&cw, nil).ToAggregate()).NotTo(HaveOccurred())
		Expect(ValidateDelete(&cw).ToAggregate()).NotTo(HaveOccurred())
	})

	It("Test validate invalid trait", func() {
		cw := baseCase
		cw.Spec.Replicas = pointer.Int32Ptr(-5)
		Expect(ValidateCreate(&cw).ToAggregate()).To(HaveOccurred())
		Expect(ValidateUpdate(&cw, nil).ToAggregate()).To(HaveOccurred())
		Expect(len(ValidateCreate(&cw))).Should(Equal(2))
	})
})
