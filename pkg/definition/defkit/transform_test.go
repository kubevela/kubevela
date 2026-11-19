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

var _ = Describe("Transform", func() {

	Context("Transform function", func() {
		It("should create a transformed value with source and transform function", func() {
			ports := defkit.List("ports")
			transformFn := func(val any) any {
				if arr, ok := val.([]any); ok {
					return len(arr)
				}
				return 0
			}

			transformed := defkit.Transform(ports, transformFn)

			Expect(transformed).NotTo(BeNil())
			Expect(transformed.Source()).To(Equal(ports))
			Expect(transformed.TransformFn()).NotTo(BeNil())
		})

		It("should apply transform function correctly", func() {
			ports := defkit.List("ports")
			transformFn := func(val any) any {
				if arr, ok := val.([]any); ok {
					return len(arr)
				}
				return 0
			}

			transformed := defkit.Transform(ports, transformFn)

			// Verify the transform function works as expected
			fn := transformed.TransformFn()
			testData := []any{1, 2, 3}
			result := fn(testData)
			Expect(result).To(Equal(3))
		})

		It("should implement Value interface", func() {
			ports := defkit.List("ports")
			transformFn := func(val any) any { return val }

			transformed := defkit.Transform(ports, transformFn)

			// TransformedValue should be usable as a Value
			var _ defkit.Value = transformed
		})
	})

	Context("HasExposedPorts condition", func() {
		It("should create a condition that checks for exposed ports", func() {
			ports := defkit.List("ports")

			condition := defkit.HasExposedPorts(ports)

			Expect(condition).NotTo(BeNil())
			// Verify we can get the ports parameter back
			hasExposedCond, ok := condition.(*defkit.HasExposedPortsCondition)
			Expect(ok).To(BeTrue())
			Expect(hasExposedCond.Ports()).To(Equal(ports))
		})

		It("should implement Condition interface", func() {
			ports := defkit.List("ports")

			condition := defkit.HasExposedPorts(ports)

			var _ defkit.Condition = condition
		})
	})
})
