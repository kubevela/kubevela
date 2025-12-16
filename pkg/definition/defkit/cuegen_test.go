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

var _ = Describe("CUEGenerator", func() {
	var gen *defkit.CUEGenerator

	BeforeEach(func() {
		gen = defkit.NewCUEGenerator()
	})

	Describe("GenerateParameterSchema", func() {
		It("should generate CUE for string parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.String("image").Required().Description("Container image"),
					defkit.String("tag").Default("latest").Description("Image tag"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("// +usage=Container image"))
			Expect(cue).To(ContainSubstring("image: string"))
			Expect(cue).To(ContainSubstring("// +usage=Image tag"))
			Expect(cue).To(ContainSubstring(`tag: *"latest" | string`))
		})

		It("should generate CUE for integer parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.Int("replicas").Default(1).Description("Number of replicas"),
					defkit.Int("port").Required(),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("replicas: *1 | int"))
			Expect(cue).To(ContainSubstring("port: int"))
		})

		It("should generate CUE for boolean parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.Bool("enabled").Default(true),
					defkit.Bool("debug").Default(false),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("enabled: *true | bool"))
			Expect(cue).To(ContainSubstring("debug: *false | bool"))
		})

		It("should generate CUE for array parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.StringList("cmd").Description("Commands"),
					defkit.List("env").Description("Environment variables"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("cmd?: [...string]"))
			// List() creates an untyped array which generates [..._]
			Expect(cue).To(ContainSubstring("env?: [..._]"))
		})

		It("should generate CUE for object parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.Object("config").Description("Configuration object"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("config?: {...}"))
		})

		It("should generate CUE for string-key map parameters", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.StringKeyMap("labels").Description("Labels"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("labels?: [string]: string"))
		})

		It("should mark required parameters without ?", func() {
			comp := defkit.NewComponent("test").
				Params(
					defkit.String("required").Required(),
					defkit.String("optional"),
				)

			cue := gen.GenerateParameterSchema(comp)

			Expect(cue).To(ContainSubstring("required: string"))
			Expect(cue).To(ContainSubstring("optional?: string"))
		})
	})

	Describe("GenerateFullDefinition", func() {
		It("should generate complete CUE definition", func() {
			comp := defkit.NewComponent("webservice").
				Description("Web service component").
				Workload("apps/v1", "Deployment").
				Params(
					defkit.String("image").Required(),
				)

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring(`webservice: {`))
			Expect(cue).To(ContainSubstring(`type: "component"`))
			Expect(cue).To(ContainSubstring(`description: "Web service component"`))
			Expect(cue).To(ContainSubstring(`apiVersion: "apps/v1"`))
			Expect(cue).To(ContainSubstring(`kind:       "Deployment"`))
			Expect(cue).To(ContainSubstring(`type: "deployments.apps"`))
			Expect(cue).To(ContainSubstring(`parameter: {`))
		})

		It("should include status when defined", func() {
			comp := defkit.NewComponent("test").
				Workload("apps/v1", "Deployment").
				CustomStatus("message: \"Ready\"").
				HealthPolicy("isHealth: true")

			cue := gen.GenerateFullDefinition(comp)

			Expect(cue).To(ContainSubstring("status: {"))
			Expect(cue).To(ContainSubstring("customStatus:"))
			Expect(cue).To(ContainSubstring("healthPolicy:"))
		})

		It("should infer correct workload types", func() {
			testCases := []struct {
				apiVersion   string
				kind         string
				expectedType string
			}{
				{"apps/v1", "Deployment", "deployments.apps"},
				{"apps/v1", "StatefulSet", "statefulsets.apps"},
				{"apps/v1", "DaemonSet", "daemonsets.apps"},
				{"batch/v1", "Job", "jobs.batch"},
				{"batch/v1", "CronJob", "cronjobs.batch"},
			}

			for _, tc := range testCases {
				comp := defkit.NewComponent("test").
					Workload(tc.apiVersion, tc.kind)

				cue := gen.GenerateFullDefinition(comp)
				Expect(cue).To(ContainSubstring(tc.expectedType),
					"Expected workload type %s for %s/%s", tc.expectedType, tc.apiVersion, tc.kind)
			}
		})
	})
})
