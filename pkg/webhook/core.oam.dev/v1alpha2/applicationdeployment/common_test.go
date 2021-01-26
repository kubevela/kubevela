package applicationdeployment

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
)

var _ = Describe("Application Deployment Common Function Test", func() {
	BeforeEach(func() {

	})

	Context("Test Find Common Component Function", func() {
		var targetApp, sourceApp *v1alpha2.Application

		BeforeEach(func() {
			targetApp = &v1alpha2.Application{
				Spec: v1alpha2.ApplicationSpec{
					Components: []v1alpha2.ApplicationComponent{},
				},
			}
			sourceApp = &v1alpha2.Application{
				Spec: v1alpha2.ApplicationSpec{
					Components: []v1alpha2.ApplicationComponent{},
				},
			}
		})

		It("Test has one common component", func() {
			fillApplication(&targetApp.Spec, []string{"a", "b", "c"})
			fillApplication(&sourceApp.Spec, []string{"c"})
			common := FindCommonComponent(targetApp, sourceApp)
			Expect(common).Should(BeEquivalentTo([]string{"c"}))
		})

		It("Test has one components", func() {
			fillApplication(&targetApp.Spec, []string{"a", "b", "c"})
			fillApplication(&sourceApp.Spec, []string{"d", "c"})
			common := FindCommonComponent(targetApp, sourceApp)
			Expect(common).Should(BeEquivalentTo([]string{"c"}))
		})

		It("Test has more than 1 common component", func() {
			fillApplication(&targetApp.Spec, []string{"a", "b", "c"})
			fillApplication(&sourceApp.Spec, []string{"b", "c"})
			common := FindCommonComponent(targetApp, sourceApp)
			Expect(common).Should(BeEquivalentTo([]string{"b", "c"}))
		})

		It("Test has more than 1 common component", func() {
			fillApplication(&targetApp.Spec, []string{"a", "b", "c"})
			fillApplication(&sourceApp.Spec, []string{"a", "c", "d", "e"})
			common := FindCommonComponent(targetApp, sourceApp)
			Expect(common).Should(BeEquivalentTo([]string{"a", "c"}))
		})

		It("Test there is no source application", func() {
			fillApplication(&targetApp.Spec, []string{"a", "b", "c"})
			common := FindCommonComponent(targetApp, nil)
			Expect(common).Should(BeEquivalentTo([]string{"a", "b", "c"}))
		})
	})
})

func fillApplication(app *v1alpha2.ApplicationSpec, componentNames []string) {
	for _, name := range componentNames {
		app.Components = append(app.Components, v1alpha2.ApplicationComponent{
			WorkloadType: name,
		})
	}
}
