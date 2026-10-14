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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func modify(kind, name string, fields ...FieldChange) ResourceChange {
	return ResourceChange{APIVersion: "apps/v1", Kind: kind, Namespace: "team-a", Name: name, Action: ActionModify, Fields: fields}
}

func field(path string, from, to any) FieldChange { return FieldChange{Path: path, From: from, To: to} }

func rulesOf(r Risk) []string {
	names := []string{}
	for _, rule := range r.Rules {
		names = append(names, rule.Rule)
	}
	return names
}

// These are the classifier's golden cases: one per rule in the KEP, plus
// the combinations that decide precedence.
var _ = DescribeTable("risk classifier",
	func(cs ChangeSet, wantClass string, wantRules ...string) {
		r := classify(cs)
		Expect(r.Class).To(Equal(wantClass))
		Expect(rulesOf(r)).To(ConsistOf(wantRules))
	},
	Entry("new application with plain resources is LOW",
		ChangeSet{NewApplication: true, Resources: []ResourceChange{{Kind: "Deployment", Name: "web", Action: ActionAdd}, {Kind: "Service", Name: "web", Action: ActionAdd}}},
		RiskLow, "new-application"),
	Entry("new application that adds a Secret is HIGH",
		ChangeSet{NewApplication: true, Resources: []ResourceChange{{Kind: "Deployment", Name: "web", Action: ActionAdd}, {Kind: "Secret", Name: "creds", Action: ActionAdd}}},
		RiskHigh, "new-application", "security-surface"),
	Entry("image tag only is LOW",
		ChangeSet{Resources: []ResourceChange{modify("Deployment", "web", field("spec.template.spec.containers[0].image", "nginx:1.26", "nginx:1.27"))}},
		RiskLow, "image-tag"),
	Entry("image repository change is HIGH",
		ChangeSet{Resources: []ResourceChange{modify("Deployment", "web", field("spec.template.spec.containers[0].image", "nginx:1.26", "evil.io/nginx:1.26"))}},
		RiskHigh, "image-repository"),
	Entry("scaling is LOW",
		ChangeSet{Resources: []ResourceChange{modify("Deployment", "web", field("spec.replicas", 2, 5))}},
		RiskLow, "scaling"),
	Entry("env and config are MEDIUM",
		ChangeSet{Resources: []ResourceChange{modify("Deployment", "web", field("spec.template.spec.containers[0].env[0].value", "a", "b"), field("spec.template.spec.containers[0].resources.limits.cpu", "1", "2"))}},
		RiskMedium, "config", "config"),
	Entry("addition to an existing application is MEDIUM",
		ChangeSet{Resources: []ResourceChange{{Kind: "ConfigMap", Name: "cfg", Action: ActionAdd}}, ComponentsAdded: []string{"cache"}},
		RiskMedium, "addition", "component-added"),
	Entry("any deletion is HIGH",
		ChangeSet{Resources: []ResourceChange{{Kind: "Deployment", Name: "old", Action: ActionRemove}}, ComponentsRemoved: []string{"old"}},
		RiskHigh, "deletion", "component-removed"),
	Entry("immutable selector change is a replace and HIGH",
		ChangeSet{Resources: []ResourceChange{modify("Deployment", "web", field("spec.selector.matchLabels.app", "a", "b"))}},
		RiskHigh, "replace"),
	Entry("placement change is HIGH",
		ChangeSet{PlacementChanged: true, Resources: []ResourceChange{modify("Deployment", "web", field("spec.replicas", 1, 2))}},
		RiskHigh, "placement-changed", "scaling"),
	Entry("security surface fields are HIGH",
		ChangeSet{Resources: []ResourceChange{modify("Deployment", "web", field("spec.template.spec.serviceAccountName", "default", "admin"))}},
		RiskHigh, "security-surface"),
	Entry("Service exposed to the outside is HIGH",
		ChangeSet{Resources: []ResourceChange{{Kind: "Service", Name: "web", Action: ActionModify, Fields: []FieldChange{field("spec.type", "ClusterIP", "LoadBalancer")}}}},
		RiskHigh, "security-surface"),
	Entry("stateful change is HIGH",
		ChangeSet{Resources: []ResourceChange{{Kind: "StatefulSet", Name: "db", Action: ActionModify, Fields: []FieldChange{field("spec.replicas", 1, 3)}}}},
		RiskHigh, "stateful", "scaling"),
	Entry("removed gate is HIGH even with a LOW change",
		ChangeSet{GateRemoved: true, WorkflowChanged: true, Resources: []ResourceChange{modify("Deployment", "web", field("spec.replicas", 1, 2))}},
		RiskHigh, "gate-removed", "scaling"),
	Entry("unrecognised field is MEDIUM, never LOW",
		ChangeSet{Resources: []ResourceChange{modify("Deployment", "web", field("spec.strategy.type", "RollingUpdate", "Recreate"))}},
		RiskMedium, "unclassified"),
	Entry("truncated field list is MEDIUM at least",
		ChangeSet{Resources: []ResourceChange{{Kind: "Deployment", Name: "web", Action: ActionModify, FieldsTruncated: true}}},
		RiskMedium, "unclassified"),
	Entry("policy change on an existing application is MEDIUM",
		ChangeSet{PoliciesChanged: []string{"override-env"}},
		RiskMedium, "policy-changed"),
	Entry("no change is LOW with no rules",
		ChangeSet{},
		RiskLow),
)

var _ = Describe("change set", func() {
	obj := func(kind, name string, spec map[string]any) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "apps/v1", "kind": kind,
			"metadata": map[string]any{"name": name, "namespace": "team-a", "labels": map[string]any{"app.oam.dev/appRevision": "x-v1"}},
			"spec":     spec,
		}}
	}

	It("diffs leaves, ignores revision bookkeeping and redacts secret-like leaves", func() {
		oldObj := obj("Deployment", "web", map[string]any{"replicas": int64(1), "template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{"image": "nginx:1.26", "env": []any{map[string]any{"name": "TOKEN", "value": "a"}}}}}}})
		newObj := obj("Deployment", "web", map[string]any{"replicas": int64(2), "template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{"image": "nginx:1.27", "env": []any{map[string]any{"name": "TOKEN", "value": "b"}}}}}}})
		newObj.SetLabels(map[string]string{"app.oam.dev/appRevision": "x-v2"})
		rc := compareObjects(oldObj, newObj)
		Expect(rc).NotTo(BeNil())
		Expect(rc.Action).To(Equal(ActionModify))
		paths := []string{}
		for _, f := range rc.Fields {
			paths = append(paths, f.Path)
		}
		Expect(paths).To(ConsistOf("spec.replicas", "spec.template.spec.containers[0].image", "spec.template.spec.containers[0].env[0].value"))
		Expect(compareObjects(oldObj, oldObj)).To(BeNil())
		Expect(compareObjects(nil, newObj).Action).To(Equal(ActionAdd))
		Expect(compareObjects(oldObj, nil).Action).To(Equal(ActionRemove))
	})

	It("detects a removed approval gate and placement changes", func() {
		gated := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Workflow: &v1beta1.Workflow{Steps: stepsOf("suspend", "deploy")}}}
		open := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Workflow: &v1beta1.Workflow{Steps: stepsOf("deploy", "suspend")}}}
		Expect(hasGate(gated)).To(BeTrue())
		Expect(hasGate(open)).To(BeFalse(), "a suspend after the first deploy is not a gate")
		Expect(hasGate(&v1beta1.Application{})).To(BeFalse())

		a := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Policies: []v1beta1.AppPolicy{{Name: "t", Type: "topology", Properties: raw(`{"clusters":["local"]}`)}}}}
		b := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Policies: []v1beta1.AppPolicy{{Name: "t", Type: "topology", Properties: raw(`{"clusters":["prod"]}`)}}}}
		Expect(placementChanged(a, b)).To(BeTrue())
		Expect(placementChanged(a, a)).To(BeFalse())
		Expect(policiesChanged(a, b)).To(Equal([]string{"t"}))

		cs := buildChangeSet(&rendered{app: gated, objects: map[string]*unstructured.Unstructured{}}, &rendered{app: open, objects: map[string]*unstructured.Unstructured{}})
		Expect(cs.GateRemoved).To(BeTrue())
		Expect(cs.WorkflowChanged).To(BeTrue())
		Expect(cs.NewApplication).To(BeFalse())
	})

	It("names added and removed components", func() {
		a := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []oamcommon.ApplicationComponent{{Name: "web"}, {Name: "old"}}}}
		b := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []oamcommon.ApplicationComponent{{Name: "web"}, {Name: "cache"}}}}
		added, removed := componentDelta(a, b)
		Expect(added).To(Equal([]string{"cache"}))
		Expect(removed).To(Equal([]string{"old"}))
	})

	It("compares image references by repository", func() {
		Expect(sameRepository("nginx:1.26", "nginx:1.27")).To(BeTrue())
		Expect(sameRepository("ghcr.io/a/app:1", "ghcr.io/a/app@sha256:abc")).To(BeTrue())
		Expect(sameRepository("localhost:5000/app:1", "localhost:5000/app:2")).To(BeTrue())
		Expect(sameRepository("ghcr.io/a/app:1", "docker.io/a/app:1")).To(BeFalse())
	})
})
