/*
Copyright 2026 The KubeVela Authors.

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

package mcp

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
)

// clusterWithInstanceTemplate emits a primary DBCluster and an auxiliary
// DBInstance, the shape whose health rule must cover both.
const clusterWithInstanceTemplate = `output: {
	apiVersion: "rds.services.k8s.aws/v1alpha1"
	kind:       "DBCluster"
	spec: engine: "aurora-postgresql"
}
outputs: instance: {
	apiVersion: "rds.services.k8s.aws/v1alpha1"
	kind:       "DBInstance"
	spec: dbInstanceClass: parameter.instanceClass
}
parameter: instanceClass: string
`

func clusterDefinition(healthPolicy string) *v1beta1.ComponentDefinition {
	def := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "aurora", Namespace: types.DefaultKubeVelaNS, ResourceVersion: "5"},
		Spec: v1beta1.ComponentDefinitionSpec{
			Workload:  oamcommon.WorkloadTypeDescriptor{Definition: oamcommon.WorkloadGVK{APIVersion: "rds.services.k8s.aws/v1alpha1", Kind: "DBCluster"}},
			Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: clusterWithInstanceTemplate}},
		},
	}
	if healthPolicy != "" {
		def.Spec.Status = &oamcommon.Status{HealthPolicy: healthPolicy}
	}
	return def
}

// producerCUE renders a DBCluster plus a FieldExport into the Secret the
// parameter names, the way vela def gen-module writes a definition.
const producerCUE = `"db-with-export": {
	type: "component"
	annotations: {}
	labels: {}
	description: "A database cluster that exports its endpoint"
	attributes: workload: definition: {
		apiVersion: "rds.services.k8s.aws/v1alpha1"
		kind:       "DBCluster"
	}
}
template: {
	output: {
		apiVersion: "rds.services.k8s.aws/v1alpha1"
		kind:       "DBCluster"
		metadata: name: context.name
		spec: engine: "aurora-postgresql"
	}
	outputs: "conn-host": {
		apiVersion: "services.k8s.aws/v1alpha1"
		kind:       "FieldExport"
		metadata: name: context.name + "-conn-host"
		spec: {
			from: {
				path: ".status.endpoint"
				resource: {group: "rds.services.k8s.aws", kind: "DBCluster", name: context.name}
			}
			to: {kind: "secret", name: parameter.connectionSecret, key: "host"}
		}
	}
	parameter: connectionSecret: string
}
`

// consumerCUE renders a Deployment whose env reads a key of the Secret the
// parameter names.
const consumerCUE = `"reader": {
	type: "component"
	annotations: {}
	labels: {}
	description: "A workload that reads a Secret"
	attributes: workload: definition: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
	}
}
template: {
	output: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: context.name
		spec: {
			selector: matchLabels: app: context.name
			template: {
				metadata: labels: app: context.name
				spec: containers: [{
					name:  "app"
					image: parameter.image
					env: [{
						name: "DB_HOST"
						valueFrom: secretKeyRef: {name: parameter.secret, key: "host"}
					}]
				}]
			}
		}
	}
	parameter: {
		image:  string
		secret: string
	}
}
`

func dependencyDoc(dependsOn string) string {
	return fmt.Sprintf(`apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: shop
  namespace: team-a
spec:
  components:
  - name: orders-db
    type: db-with-export
    properties:
      connectionSecret: orders-db
  - name: orders-api
    type: reader
%s    properties:
      image: api:1
      secret: orders-db
`, dependsOn)
}

var _ = Describe("definition and dry-run lints", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	Describe("explain_definition on a health rule that ignores the auxiliary outputs", func() {
		It("warns when the health policy reads only the primary output", func() {
			h = startHarness(Options{}, append(platformFixtures(), clusterDefinition(`isHealth: context.output.status.conditions != _|_`))...)
			var out ExplainDefinitionOutput
			res := h.call("explain_definition", map[string]any{"name": "aurora"}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Findings).To(ConsistOf(SatisfyAll(
				HaveField("Severity", SeverityWarning),
				HaveField("Message", ContainSubstring("the health policy reads context.output only, so the component is healthy before instance (DBInstance) is")),
				HaveField("Evidence", "ComponentDefinition vela-system/aurora resourceVersion 5"),
			)))
		})

		It("warns when there is no health policy at all", func() {
			h = startHarness(Options{}, append(platformFixtures(), clusterDefinition(""))...)
			var out ExplainDefinitionOutput
			res := h.call("explain_definition", map[string]any{"name": "aurora"}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Findings).To(ConsistOf(HaveField("Message", ContainSubstring("declares no health policy, so health falls back to the built-in check on the primary resource; its 1 auxiliary outputs (instance (DBInstance)) are not judged"))))
		})

		It("says nothing when the policy reads every auxiliary output", func() {
			h = startHarness(Options{}, append(platformFixtures(), clusterDefinition(`isHealth: context.output.status.ready && context.outputs.instance.status.ready`))...)
			var out ExplainDefinitionOutput
			res := h.call("explain_definition", map[string]any{"name": "aurora"}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Findings).To(BeEmpty())
		})

		It("names the outputs a partial policy leaves unread", func() {
			def := clusterDefinition(`isHealth: context.outputs.instance.status.ready`)
			def.Spec.Schematic.CUE.Template = clusterWithInstanceTemplate + "outputs: subnets: {\n\tapiVersion: \"rds.services.k8s.aws/v1alpha1\"\n\tkind: \"DBSubnetGroup\"\n}\n"
			h = startHarness(Options{}, append(platformFixtures(), def)...)
			var out ExplainDefinitionOutput
			res := h.call("explain_definition", map[string]any{"name": "aurora"}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Findings).To(ConsistOf(HaveField("Message", ContainSubstring("leaves 1 of its 2 auxiliary outputs unread, so the component is healthy before subnets (DBSubnetGroup) is"))))
		})

		It("says nothing for a definition with a single output", func() {
			h = startHarness(Options{}, platformFixtures()...)
			var out ExplainDefinitionOutput
			res := h.call("explain_definition", map[string]any{"name": "webservice"}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Findings).To(BeEmpty())
		})
	})

	Describe("dryrun_application on a consumer without dependsOn", func() {
		It("warns when a rendered workload reads a Secret an exporter of another component fills", func() {
			h = startHarness(Options{}, platformFixtures()...)
			var out DryRunOutput
			res := h.call("dryrun_application", map[string]any{"application": dependencyDoc(""), "definitions": []string{producerCUE, consumerCUE}}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Resources).To(HaveLen(3), toJSON(out.Resources))
			Expect(out.Findings).To(ConsistOf(SatisfyAll(
				HaveField("Severity", SeverityWarning),
				HaveField("Message", `component "orders-api": Deployment orders-api reads Secret "orders-db" key "host" that component "orders-db" produces, and declares no dependsOn on it; the default workflow applies components in parallel, so the consumer can start before the object or its keys exist`),
				HaveField("Evidence", `rendered Deployment orders-api of component "orders-api"`),
			)))
		})

		It("says nothing when dependsOn is declared", func() {
			h = startHarness(Options{}, platformFixtures()...)
			var out DryRunOutput
			res := h.call("dryrun_application", map[string]any{"application": dependencyDoc("    dependsOn: [orders-db]\n"), "definitions": []string{producerCUE, consumerCUE}}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Findings).To(BeEmpty(), toJSON(out.Findings))
		})

		It("says nothing when nothing in the Application produces what the workload reads", func() {
			h = startHarness(Options{}, platformFixtures()...)
			var out DryRunOutput
			res := h.call("dryrun_application", map[string]any{"application": shopDoc("nginx:1.27", 1, "")}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Findings).To(BeEmpty())
		})

		It("matches a producer only in the consumer's namespace", func() {
			app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []oamcommon.ApplicationComponent{{Name: "db"}, {Name: "api"}}}}
			secret := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": "conn", "namespace": "other"}}}
			deploy := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "api", "namespace": "team-a"},
				"spec": map[string]any{"template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{"name": "c", "envFrom": []any{map[string]any{"secretRef": map[string]any{"name": "conn"}}}}}}}}}}
			resources := []RenderedResource{
				{Component: "db", APIVersion: "v1", Kind: "Secret", Namespace: "other", Name: "conn"},
				{Component: "api", APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api"},
			}
			objects := map[string]*unstructured.Unstructured{
				resourceKey("v1", "Secret", "other", "conn"):          secret,
				resourceKey("apps/v1", "Deployment", "team-a", "api"): deploy,
			}
			Expect(dependencyFindings(app, resources, objects)).To(BeEmpty(), "a same-named Secret in another namespace is not what the workload reads")
			resources[0].Namespace = "team-a"
			objects[resourceKey("v1", "Secret", "team-a", "conn")] = secret
			Expect(dependencyFindings(app, resources, objects)).To(HaveLen(1))
		})
	})
})

var _ = DescribeTable("outputsRead finds the auxiliary outputs a health policy reads",
	func(policy string, wantNames []string, wantWhole bool) {
		names, whole := outputsRead(policy)
		got := make([]string, 0, len(names))
		for n := range names {
			got = append(got, n)
		}
		Expect(got).To(ConsistOf(wantNames))
		Expect(whole).To(Equal(wantWhole))
	},
	Entry("dot notation", `isHealth: context.outputs.instance.status.ready`, []string{"instance"}, false),
	Entry("bracket notation", `isHealth: context.outputs["conn-host"].status.ready`, []string{"conn-host"}, false),
	Entry("two outputs, both notations", `isHealth: context.outputs.instance.ok && context.outputs["subnet-group"].ok`, []string{"instance", "subnet-group"}, false),
	Entry("a comprehension over the whole map", `_items: [ for k, v in (*context.outputs | {}) if k =~ "^instance-" { v } ]`, []string{}, true),
	Entry("the whole map by reference", `isHealth: len(context.outputs) > 0`, []string{}, true),
	Entry("the primary output only", `isHealth: context.output.status.readyReplicas == context.output.spec.replicas`, []string{}, false),
	Entry("a constant", `isHealth: true`, []string{}, false),
	Entry("a name that merely starts with outputs", `isHealth: context.outputsCount > 0`, []string{}, false),
)
