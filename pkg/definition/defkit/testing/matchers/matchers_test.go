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

package matchers_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
	. "github.com/oam-dev/kubevela/pkg/definition/defkit/testing/matchers"
)

var _ = Describe("Resource Matchers", func() {

	Describe("BeDeployment", func() {
		It("should match a Deployment resource", func() {
			r := defkit.NewResource("apps/v1", "Deployment")
			Expect(r).To(BeDeployment())
		})

		It("should not match a Service resource", func() {
			r := defkit.NewResource("v1", "Service")
			Expect(r).NotTo(BeDeployment())
		})
	})

	Describe("BeService", func() {
		It("should match a Service resource", func() {
			r := defkit.NewResource("v1", "Service")
			Expect(r).To(BeService())
		})
	})

	Describe("BeConfigMap", func() {
		It("should match a ConfigMap resource", func() {
			r := defkit.NewResource("v1", "ConfigMap")
			Expect(r).To(BeConfigMap())
		})
	})

	Describe("BeSecret", func() {
		It("should match a Secret resource", func() {
			r := defkit.NewResource("v1", "Secret")
			Expect(r).To(BeSecret())
		})
	})

	Describe("BeIngress", func() {
		It("should match an Ingress resource", func() {
			r := defkit.NewResource("networking.k8s.io/v1", "Ingress")
			Expect(r).To(BeIngress())
		})
	})

	Describe("BeResourceOfKind", func() {
		It("should match a resource of the specified kind", func() {
			r := defkit.NewResource("batch/v1", "Job")
			Expect(r).To(BeResourceOfKind("Job"))
		})

		It("should not match a different kind", func() {
			r := defkit.NewResource("batch/v1", "CronJob")
			Expect(r).NotTo(BeResourceOfKind("Job"))
		})
	})

	Describe("HaveAPIVersion", func() {
		It("should match the correct API version", func() {
			r := defkit.NewResource("apps/v1", "Deployment")
			Expect(r).To(HaveAPIVersion("apps/v1"))
		})

		It("should not match a different API version", func() {
			r := defkit.NewResource("apps/v1", "Deployment")
			Expect(r).NotTo(HaveAPIVersion("v1"))
		})
	})

	Describe("HaveSetOp", func() {
		It("should match when Set operation exists at path", func() {
			image := defkit.String("image")
			r := defkit.NewResource("apps/v1", "Deployment").
				Set("spec.template.spec.containers[0].image", image)
			Expect(r).To(HaveSetOp("spec.template.spec.containers[0].image"))
		})

		It("should not match when Set operation does not exist", func() {
			r := defkit.NewResource("apps/v1", "Deployment")
			Expect(r).NotTo(HaveSetOp("spec.replicas"))
		})
	})

	Describe("HaveOpCount", func() {
		It("should match the correct operation count", func() {
			image := defkit.String("image")
			replicas := defkit.Int("replicas")
			r := defkit.NewResource("apps/v1", "Deployment").
				Set("spec.template.spec.containers[0].image", image).
				Set("spec.replicas", replicas)
			Expect(r).To(HaveOpCount(2))
		})

		It("should match zero operations", func() {
			r := defkit.NewResource("apps/v1", "Deployment")
			Expect(r).To(HaveOpCount(0))
		})
	})
})

var _ = Describe("Parameter Matchers", func() {

	Describe("BeRequired", func() {
		It("should match a required parameter", func() {
			p := defkit.String("image").Required()
			Expect(p).To(BeRequired())
		})

		It("should not match an optional parameter", func() {
			p := defkit.String("image")
			Expect(p).NotTo(BeRequired())
		})
	})

	Describe("BeOptional", func() {
		It("should match an optional parameter", func() {
			p := defkit.Int("replicas")
			Expect(p).To(BeOptional())
		})

		It("should not match a required parameter", func() {
			p := defkit.Int("replicas").Required()
			Expect(p).NotTo(BeOptional())
		})
	})

	Describe("HaveDefaultValue", func() {
		It("should match the correct default value for string", func() {
			p := defkit.String("image").Default("nginx:latest")
			Expect(p).To(HaveDefaultValue("nginx:latest"))
		})

		It("should match the correct default value for int", func() {
			p := defkit.Int("replicas").Default(3)
			Expect(p).To(HaveDefaultValue(3))
		})

		It("should match the correct default value for bool", func() {
			p := defkit.Bool("enabled").Default(true)
			Expect(p).To(HaveDefaultValue(true))
		})

		It("should not match when default value differs", func() {
			p := defkit.Int("replicas").Default(3)
			Expect(p).NotTo(HaveDefaultValue(5))
		})

		It("should not match when no default is set", func() {
			p := defkit.Int("replicas")
			Expect(p).NotTo(HaveDefaultValue(1))
		})
	})

	Describe("HaveDescription", func() {
		It("should match the correct description", func() {
			p := defkit.String("image").Description("Container image to use")
			Expect(p).To(HaveDescription("Container image to use"))
		})

		It("should not match when description differs", func() {
			p := defkit.String("image").Description("Container image")
			Expect(p).NotTo(HaveDescription("Image name"))
		})
	})

	Describe("HaveParamNamed", func() {
		It("should match when component has parameter", func() {
			c := defkit.NewComponent("webservice").
				Params(
					defkit.String("image").Required(),
					defkit.Int("replicas").Default(1),
				)
			Expect(c).To(HaveParamNamed("image"))
			Expect(c).To(HaveParamNamed("replicas"))
		})

		It("should not match when parameter is missing", func() {
			c := defkit.NewComponent("webservice").
				Params(defkit.String("image"))
			Expect(c).NotTo(HaveParamNamed("replicas"))
		})
	})
})
