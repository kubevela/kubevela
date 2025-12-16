/*
Copyright 2025 The KubeVela Authors.

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

package defkit_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("ComponentDefinition", func() {

	Describe("NewComponent", func() {
		It("should create a component with name", func() {
			c := defkit.NewComponent("webservice")
			Expect(c.GetName()).To(Equal("webservice"))
			Expect(c.GetParams()).To(BeEmpty())
		})
	})

	Describe("Component Configuration", func() {
		It("should set description", func() {
			c := defkit.NewComponent("webservice").
				Description("Describes long-running, scalable, containerized services")
			Expect(c.GetDescription()).To(Equal("Describes long-running, scalable, containerized services"))
		})

		It("should set workload type", func() {
			c := defkit.NewComponent("webservice").
				Workload("apps/v1", "Deployment")
			Expect(c.GetWorkload().APIVersion()).To(Equal("apps/v1"))
			Expect(c.GetWorkload().Kind()).To(Equal("Deployment"))
		})

		It("should add parameters", func() {
			image := defkit.String("image").Required()
			replicas := defkit.Int("replicas").Default(1)
			c := defkit.NewComponent("webservice").
				Params(image, replicas)
			Expect(c.GetParams()).To(HaveLen(2))
			Expect(c.GetParams()[0].Name()).To(Equal("image"))
			Expect(c.GetParams()[1].Name()).To(Equal("replicas"))
		})

		It("should set template function", func() {
			var templateCalled bool
			c := defkit.NewComponent("webservice").
				Template(func(tpl *defkit.Template) {
					templateCalled = true
				})
			Expect(c.GetTemplate()).NotTo(BeNil())
			c.GetTemplate()(defkit.NewTemplate())
			Expect(templateCalled).To(BeTrue())
		})
	})

	Describe("Template", func() {
		It("should create a new template", func() {
			tpl := defkit.NewTemplate()
			Expect(tpl).NotTo(BeNil())
			Expect(tpl.GetOutput()).To(BeNil())
			Expect(tpl.GetOutputs()).To(BeEmpty())
		})

		It("should set primary output", func() {
			tpl := defkit.NewTemplate()
			r := defkit.NewResource("apps/v1", "Deployment")
			tpl.Output(r)
			Expect(tpl.GetOutput()).To(Equal(r))
		})

		It("should return existing output when called without args", func() {
			tpl := defkit.NewTemplate()
			r := defkit.NewResource("apps/v1", "Deployment")
			tpl.Output(r)
			Expect(tpl.Output()).To(Equal(r))
		})

		It("should set auxiliary outputs", func() {
			tpl := defkit.NewTemplate()
			svc := defkit.NewResource("v1", "Service")
			cm := defkit.NewResource("v1", "ConfigMap")
			tpl.Outputs("service", svc)
			tpl.Outputs("config", cm)
			Expect(tpl.GetOutputs()).To(HaveLen(2))
			Expect(tpl.Outputs("service")).To(Equal(svc))
			Expect(tpl.Outputs("config")).To(Equal(cm))
		})
	})

	Describe("Full Component Example", func() {
		It("should build a complete webservice component", func() {
			image := defkit.String("image").Required().Description("Container image")
			replicas := defkit.Int("replicas").Default(1)
			port := defkit.Int("port").Default(80)

			c := defkit.NewComponent("webservice").
				Description("Web service component").
				Workload("apps/v1", "Deployment").
				Params(image, replicas, port).
				Template(func(tpl *defkit.Template) {
					vela := defkit.VelaCtx()
					tpl.Output(
						defkit.NewResource("apps/v1", "Deployment").
							Set("metadata.name", vela.Name()).
							Set("spec.replicas", replicas).
							Set("spec.template.spec.containers[0].image", image).
							Set("spec.template.spec.containers[0].ports[0].containerPort", port),
					)
					tpl.Outputs("service",
						defkit.NewResource("v1", "Service").
							Set("metadata.name", vela.Name()).
							Set("spec.ports[0].port", port),
					)
				})

			Expect(c.GetName()).To(Equal("webservice"))
			Expect(c.GetParams()).To(HaveLen(3))
			Expect(c.GetWorkload().Kind()).To(Equal("Deployment"))

			// Execute template to verify it builds correctly
			tpl := defkit.NewTemplate()
			c.GetTemplate()(tpl)
			Expect(tpl.GetOutput()).NotTo(BeNil())
			Expect(tpl.GetOutput().Kind()).To(Equal("Deployment"))
			Expect(tpl.GetOutput().Ops()).To(HaveLen(4))
			Expect(tpl.GetOutputs()).To(HaveKey("service"))
		})
	})
})
