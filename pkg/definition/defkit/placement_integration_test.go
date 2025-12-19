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
	"github.com/oam-dev/kubevela/pkg/definition/defkit/placement"
)

var _ = Describe("Placement Integration", func() {

	Describe("ComponentDefinition Placement", func() {
		It("should add RunOn conditions", func() {
			c := defkit.NewComponent("eks-only").
				RunOn(placement.Label("provider").Eq("aws"))

			Expect(c.HasPlacement()).To(BeTrue())
			Expect(c.GetRunOn()).To(HaveLen(1))
			Expect(c.GetNotRunOn()).To(BeEmpty())
		})

		It("should add NotRunOn conditions", func() {
			c := defkit.NewComponent("no-vclusters").
				NotRunOn(placement.Label("cluster-type").Eq("vcluster"))

			Expect(c.HasPlacement()).To(BeTrue())
			Expect(c.GetRunOn()).To(BeEmpty())
			Expect(c.GetNotRunOn()).To(HaveLen(1))
		})

		It("should combine multiple RunOn calls", func() {
			c := defkit.NewComponent("multi-constraint").
				RunOn(placement.Label("provider").Eq("aws")).
				RunOn(placement.Label("env").In("prod", "staging"))

			Expect(c.GetRunOn()).To(HaveLen(2))
		})

		It("should support both RunOn and NotRunOn", func() {
			c := defkit.NewComponent("complex-placement").
				RunOn(placement.Label("provider").Eq("aws")).
				NotRunOn(placement.Label("cluster-type").Eq("vcluster"))

			Expect(c.HasPlacement()).To(BeTrue())
			Expect(c.GetRunOn()).To(HaveLen(1))
			Expect(c.GetNotRunOn()).To(HaveLen(1))
		})

		It("should return correct PlacementSpec", func() {
			c := defkit.NewComponent("with-placement").
				RunOn(placement.Label("provider").Eq("aws")).
				NotRunOn(placement.Label("cluster-type").Eq("vcluster"))

			spec := c.GetPlacement()
			Expect(spec.RunOn).To(HaveLen(1))
			Expect(spec.NotRunOn).To(HaveLen(1))
		})

		It("should chain with other builder methods", func() {
			c := defkit.NewComponent("full-component").
				Description("A component with placement").
				Workload("apps/v1", "Deployment").
				RunOn(placement.Label("provider").Eq("aws")).
				Params(defkit.String("image").Required())

			Expect(c.GetDescription()).To(Equal("A component with placement"))
			Expect(c.GetWorkload().Kind()).To(Equal("Deployment"))
			Expect(c.HasPlacement()).To(BeTrue())
			Expect(c.GetParams()).To(HaveLen(1))
		})
	})

	Describe("TraitDefinition Placement", func() {
		It("should add RunOn conditions", func() {
			t := defkit.NewTrait("eks-scaler").
				RunOn(placement.Label("provider").Eq("aws"))

			Expect(t.HasPlacement()).To(BeTrue())
			Expect(t.GetRunOn()).To(HaveLen(1))
		})

		It("should add NotRunOn conditions", func() {
			t := defkit.NewTrait("no-vclusters").
				NotRunOn(placement.Label("cluster-type").Eq("vcluster"))

			Expect(t.HasPlacement()).To(BeTrue())
			Expect(t.GetNotRunOn()).To(HaveLen(1))
		})

		It("should chain with other builder methods", func() {
			t := defkit.NewTrait("full-trait").
				Description("A trait with placement").
				AppliesTo("deployments.apps").
				RunOn(placement.Label("env").Eq("prod")).
				Params(defkit.Int("replicas").Default(1))

			Expect(t.GetDescription()).To(Equal("A trait with placement"))
			Expect(t.GetAppliesToWorkloads()).To(ContainElement("deployments.apps"))
			Expect(t.HasPlacement()).To(BeTrue())
			Expect(t.GetParams()).To(HaveLen(1))
		})
	})

	Describe("PolicyDefinition Placement", func() {
		It("should add RunOn conditions", func() {
			p := defkit.NewPolicy("eks-topology").
				RunOn(placement.Label("provider").Eq("aws"))

			Expect(p.HasPlacement()).To(BeTrue())
			Expect(p.GetRunOn()).To(HaveLen(1))
		})

		It("should add NotRunOn conditions", func() {
			p := defkit.NewPolicy("no-vclusters").
				NotRunOn(placement.Label("cluster-type").Eq("vcluster"))

			Expect(p.HasPlacement()).To(BeTrue())
			Expect(p.GetNotRunOn()).To(HaveLen(1))
		})

		It("should chain with other builder methods", func() {
			p := defkit.NewPolicy("full-policy").
				Description("A policy with placement").
				RunOn(placement.Label("provider").In("aws", "gcp")).
				Params(defkit.String("target").Required())

			Expect(p.GetDescription()).To(Equal("A policy with placement"))
			Expect(p.HasPlacement()).To(BeTrue())
			Expect(p.GetParams()).To(HaveLen(1))
		})
	})

	Describe("WorkflowStepDefinition Placement", func() {
		It("should add RunOn conditions", func() {
			w := defkit.NewWorkflowStep("eks-deploy").
				RunOn(placement.Label("provider").Eq("aws"))

			Expect(w.HasPlacement()).To(BeTrue())
			Expect(w.GetRunOn()).To(HaveLen(1))
		})

		It("should add NotRunOn conditions", func() {
			w := defkit.NewWorkflowStep("no-vclusters").
				NotRunOn(placement.Label("cluster-type").Eq("vcluster"))

			Expect(w.HasPlacement()).To(BeTrue())
			Expect(w.GetNotRunOn()).To(HaveLen(1))
		})

		It("should chain with other builder methods", func() {
			w := defkit.NewWorkflowStep("full-step").
				Description("A workflow step with placement").
				Category("Deployment").
				RunOn(placement.Label("env").Eq("prod")).
				Params(defkit.Bool("auto").Default(false))

			Expect(w.GetDescription()).To(Equal("A workflow step with placement"))
			Expect(w.GetCategory()).To(Equal("Deployment"))
			Expect(w.HasPlacement()).To(BeTrue())
			Expect(w.GetParams()).To(HaveLen(1))
		})
	})

	Describe("Complex Placement Conditions", func() {
		It("should support All() conditions", func() {
			c := defkit.NewComponent("complex-runon").
				RunOn(placement.All(
					placement.Label("provider").Eq("aws"),
					placement.Label("env").In("prod", "staging"),
				))

			Expect(c.GetRunOn()).To(HaveLen(1))
			// Verify the condition is an AllCondition
			_, ok := c.GetRunOn()[0].(*placement.AllCondition)
			Expect(ok).To(BeTrue())
		})

		It("should support Any() conditions", func() {
			c := defkit.NewComponent("any-provider").
				RunOn(placement.Any(
					placement.Label("provider").Eq("aws"),
					placement.Label("provider").Eq("gcp"),
				))

			Expect(c.GetRunOn()).To(HaveLen(1))
			_, ok := c.GetRunOn()[0].(*placement.AnyCondition)
			Expect(ok).To(BeTrue())
		})

		It("should support Not() conditions", func() {
			c := defkit.NewComponent("not-dev").
				RunOn(placement.Not(placement.Label("env").Eq("dev")))

			Expect(c.GetRunOn()).To(HaveLen(1))
			_, ok := c.GetRunOn()[0].(*placement.NotCondition)
			Expect(ok).To(BeTrue())
		})

		It("should support Exists() conditions", func() {
			c := defkit.NewComponent("has-label").
				RunOn(placement.Label("gpu").Exists())

			Expect(c.GetRunOn()).To(HaveLen(1))
		})

		It("should support NotExists() conditions", func() {
			c := defkit.NewComponent("no-deprecated").
				NotRunOn(placement.Label("deprecated").Exists())

			Expect(c.GetNotRunOn()).To(HaveLen(1))
		})
	})

	Describe("Placement Evaluation Integration", func() {
		It("should evaluate eligible placement", func() {
			c := defkit.NewComponent("eks-only").
				RunOn(placement.Label("provider").Eq("aws")).
				NotRunOn(placement.Label("cluster-type").Eq("vcluster"))

			clusterLabels := map[string]string{
				"provider":     "aws",
				"cluster-type": "eks",
			}

			result := placement.Evaluate(c.GetPlacement(), clusterLabels)
			Expect(result.Eligible).To(BeTrue())
		})

		It("should evaluate ineligible placement - RunOn not satisfied", func() {
			c := defkit.NewComponent("eks-only").
				RunOn(placement.Label("provider").Eq("aws"))

			clusterLabels := map[string]string{
				"provider": "gcp",
			}

			result := placement.Evaluate(c.GetPlacement(), clusterLabels)
			Expect(result.Eligible).To(BeFalse())
			Expect(result.Reason).To(ContainSubstring("runOn"))
		})

		It("should evaluate ineligible placement - NotRunOn matched", func() {
			c := defkit.NewComponent("no-vclusters").
				NotRunOn(placement.Label("cluster-type").Eq("vcluster"))

			clusterLabels := map[string]string{
				"provider":     "aws",
				"cluster-type": "vcluster",
			}

			result := placement.Evaluate(c.GetPlacement(), clusterLabels)
			Expect(result.Eligible).To(BeFalse())
			Expect(result.Reason).To(ContainSubstring("notRunOn"))
		})

		It("should evaluate with no placement constraints", func() {
			c := defkit.NewComponent("anywhere")

			Expect(c.HasPlacement()).To(BeFalse())

			clusterLabels := map[string]string{
				"provider": "aws",
			}

			result := placement.Evaluate(c.GetPlacement(), clusterLabels)
			Expect(result.Eligible).To(BeTrue())
		})
	})

	Describe("Placement Validation", func() {
		BeforeEach(func() {
			// Clear registry before each test
			defkit.Clear()
		})

		It("should panic when registering definition with conflicting placement", func() {
			// Same condition in both RunOn and NotRunOn should panic
			Expect(func() {
				c := defkit.NewComponent("conflicting").
					RunOn(placement.Label("cloud").Eq("aws")).
					NotRunOn(placement.Label("cloud").Eq("aws"))
				defkit.Register(c)
			}).To(Panic())
		})

		It("should panic when RunOn Eq conflicts with NotRunOn Exists", func() {
			Expect(func() {
				c := defkit.NewComponent("conflicting-exists").
					RunOn(placement.Label("cloud").Eq("aws")).
					NotRunOn(placement.Label("cloud").Exists())
				defkit.Register(c)
			}).To(Panic())
		})

		It("should not panic for valid non-conflicting placement", func() {
			Expect(func() {
				c := defkit.NewComponent("valid-placement").
					RunOn(placement.Label("cloud").Eq("aws")).
					NotRunOn(placement.Label("env").Eq("dev"))
				defkit.Register(c)
			}).NotTo(Panic())
		})

		It("should not panic for definition without placement", func() {
			Expect(func() {
				c := defkit.NewComponent("no-placement")
				defkit.Register(c)
			}).NotTo(Panic())
		})
	})

	Describe("Real-world Scenarios", func() {
		It("should handle EKS-only component", func() {
			c := defkit.NewComponent("eks-app-mesh").
				Description("App Mesh integration for EKS").
				RunOn(
					placement.Label("provider").Eq("aws"),
					placement.Label("cluster-type").In("eks", "eks-fargate"),
				).
				NotRunOn(placement.Label("cluster-type").Eq("vcluster"))

			// EKS cluster - should be eligible
			eksLabels := map[string]string{
				"provider":     "aws",
				"cluster-type": "eks",
			}
			Expect(placement.Evaluate(c.GetPlacement(), eksLabels).Eligible).To(BeTrue())

			// vcluster on EKS - should NOT be eligible
			vclusterLabels := map[string]string{
				"provider":     "aws",
				"cluster-type": "vcluster",
			}
			Expect(placement.Evaluate(c.GetPlacement(), vclusterLabels).Eligible).To(BeFalse())

			// GCP cluster - should NOT be eligible (wrong provider)
			gcpLabels := map[string]string{
				"provider":     "gcp",
				"cluster-type": "gke",
			}
			Expect(placement.Evaluate(c.GetPlacement(), gcpLabels).Eligible).To(BeFalse())
		})

		It("should handle production-only trait", func() {
			t := defkit.NewTrait("prod-autoscaler").
				Description("HPA with production settings").
				RunOn(placement.Label("env").Eq("prod")).
				NotRunOn(placement.Label("cluster-type").Eq("vcluster"))

			prodLabels := map[string]string{
				"env":          "prod",
				"cluster-type": "eks",
			}
			Expect(placement.Evaluate(t.GetPlacement(), prodLabels).Eligible).To(BeTrue())

			devLabels := map[string]string{
				"env":          "dev",
				"cluster-type": "eks",
			}
			Expect(placement.Evaluate(t.GetPlacement(), devLabels).Eligible).To(BeFalse())
		})

		It("should handle multi-cloud policy", func() {
			p := defkit.NewPolicy("multi-cloud-topology").
				Description("Deploy across AWS and GCP").
				RunOn(placement.Any(
					placement.Label("provider").Eq("aws"),
					placement.Label("provider").Eq("gcp"),
				))

			awsLabels := map[string]string{"provider": "aws"}
			gcpLabels := map[string]string{"provider": "gcp"}
			azureLabels := map[string]string{"provider": "azure"}

			Expect(placement.Evaluate(p.GetPlacement(), awsLabels).Eligible).To(BeTrue())
			Expect(placement.Evaluate(p.GetPlacement(), gcpLabels).Eligible).To(BeTrue())
			Expect(placement.Evaluate(p.GetPlacement(), azureLabels).Eligible).To(BeFalse())
		})
	})
})
