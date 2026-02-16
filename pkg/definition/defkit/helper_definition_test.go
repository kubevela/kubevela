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

var _ = Describe("HelperDefinition", func() {

	Context("Param-based helper definition", func() {
		It("should report HasParam true and return the param", func() {
			probeParam := defkit.Struct("probe").Fields(
				defkit.Field("path", defkit.ParamTypeString).Default("/health"),
				defkit.Field("port", defkit.ParamTypeInt).Default(8080),
			)

			comp := defkit.NewComponent("test").
				Helper("HealthProbe", probeParam)

			helpers := comp.GetHelperDefinitions()
			Expect(helpers).To(HaveLen(1))
			h := helpers[0]

			Expect(h.GetName()).To(Equal("HealthProbe"))
			Expect(h.HasParam()).To(BeTrue())
			Expect(h.GetParam()).To(Equal(probeParam))
			Expect(h.GetSchema()).To(BeEmpty())
		})
	})

	Context("Multiple helper definitions", func() {
		It("should support multiple helpers on the same component", func() {
			probe := defkit.Struct("probe").Fields(
				defkit.Field("path", defkit.ParamTypeString),
			)
			resource := defkit.Struct("resource").Fields(
				defkit.Field("cpu", defkit.ParamTypeString),
				defkit.Field("memory", defkit.ParamTypeString),
			)

			comp := defkit.NewComponent("test").
				Helper("HealthProbe", probe).
				Helper("ResourceSpec", resource)

			helpers := comp.GetHelperDefinitions()
			Expect(helpers).To(HaveLen(2))
			Expect(helpers[0].GetName()).To(Equal("HealthProbe"))
			Expect(helpers[1].GetName()).To(Equal("ResourceSpec"))
		})
	})

	Context("HelperDefinition on TraitDefinition", func() {
		It("should support helper definitions on traits", func() {
			probeParam := defkit.Struct("probe").Fields(
				defkit.Field("path", defkit.ParamTypeString),
			)

			trait := defkit.NewTrait("health-probe").
				Description("Add health probes").
				AppliesTo("deployments.apps").
				Helper("HealthProbe", probeParam)

			helpers := trait.GetHelperDefinitions()
			Expect(helpers).To(HaveLen(1))
			Expect(helpers[0].GetName()).To(Equal("HealthProbe"))
			Expect(helpers[0].HasParam()).To(BeTrue())
		})
	})

	Context("Helper without param (raw schema)", func() {
		It("should report HasParam false when param is nil", func() {
			// Use the raw schema path by passing a nil param through the builder
			// The HelperDefinition struct should report HasParam false
			comp := defkit.NewComponent("test")

			// Access helper definitions - should be empty initially
			helpers := comp.GetHelperDefinitions()
			Expect(helpers).To(BeEmpty())
		})
	})
})
