package metrics

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/v1alpha1"
)

func TestMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Suite")
}

var _ = Describe("Metrics Admission controller Test", func() {
	var traitBase v1alpha1.MetricsTrait

	BeforeEach(func() {
		traitBase = v1alpha1.MetricsTrait{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mutate-hook",
				Namespace: "default",
			},
			Spec: v1alpha1.MetricsTraitSpec{
				ScrapeService: v1alpha1.ScapeServiceEndPoint{
					TargetPort: intstr.FromInt(1234),
				},
			},
		}
	})

	It("Test with fill in all default", func() {
		trait := traitBase
		want := traitBase
		want.Spec.ScrapeService.Format = SupportedFormat
		want.Spec.ScrapeService.Scheme = SupportedScheme
		want.Spec.ScrapeService.Path = DefaultMetricsPath
		want.Spec.ScrapeService.Enabled = pointer.BoolPtr(true)
		DefaultMetrics(&trait)
		Expect(trait).Should(BeEquivalentTo(want))
	})

	It("Test only fill in empty fields", func() {
		trait := traitBase
		trait.Spec.ScrapeService.Path = "not default"
		want := trait
		want.Spec.ScrapeService.Format = SupportedFormat
		want.Spec.ScrapeService.Scheme = SupportedScheme
		want.Spec.ScrapeService.Enabled = pointer.BoolPtr(true)
		DefaultMetrics(&trait)
		Expect(trait).Should(BeEquivalentTo(want))
	})

	It("Test not fill in enabled field", func() {
		trait := traitBase
		trait.Spec.ScrapeService.Enabled = pointer.BoolPtr(false)
		want := trait
		want.Spec.ScrapeService.Format = SupportedFormat
		want.Spec.ScrapeService.Scheme = SupportedScheme
		want.Spec.ScrapeService.Path = DefaultMetricsPath
		want.Spec.ScrapeService.Enabled = pointer.BoolPtr(false)
		DefaultMetrics(&trait)
		Expect(trait).Should(BeEquivalentTo(want))
	})

	It("Test validate valid trait", func() {
		trait := traitBase
		trait.Spec.ScrapeService.Format = SupportedFormat
		trait.Spec.ScrapeService.Scheme = SupportedScheme
		Expect(ValidateCreate(&trait).ToAggregate()).NotTo(HaveOccurred())
		Expect(ValidateUpdate(&trait, nil).ToAggregate()).NotTo(HaveOccurred())
		Expect(ValidateDelete(&trait).ToAggregate()).NotTo(HaveOccurred())
	})

	It("Test validate invalid trait", func() {
		trait := traitBase
		Expect(ValidateCreate(&trait).ToAggregate()).To(HaveOccurred())
		Expect(ValidateUpdate(&trait, nil).ToAggregate()).To(HaveOccurred())
		Expect(len(ValidateCreate(&trait))).Should(Equal(2))
	})
})
