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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func event(name, kind, object, eventType, reason, message string, when time.Time) *corev1.Event {
	return &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: name, Namespace: "team-a"},
		InvolvedObject: corev1.ObjectReference{Kind: kind, Name: object, Namespace: "team-a"},
		Type:           eventType,
		Reason:         reason,
		Message:        message,
		Count:          3,
		LastTimestamp:  metav1.NewTime(when),
	}
}

func explainAppFixtures() []client.Object {
	objs := append(managedFixtures(), failedApp())
	for i := range objs {
		if pod, ok := objs[i].(*corev1.Pod); ok && pod.Name == "web-abc-2" {
			pod.Status.ContainerStatuses[0].RestartCount = 4
		}
	}
	now := time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)
	return append(objs,
		event("e1", "Pod", "web-abc-2", corev1.EventTypeWarning, "BackOff", "Back-off restarting failed container web; token=abc123", now),
		event("e2", "Application", "web", corev1.EventTypeNormal, "Deployed", "workflow step deploy-web succeeded", now.Add(-time.Hour)),
		event("e3", "Deployment", "rogue", corev1.EventTypeWarning, "FailedCreate", "unrelated object", now),
	)
}

var _ = Describe("explain_application", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	It("ties components, definitions, resources, events and findings together with evidence", func() {
		h = startHarness(Options{}, explainAppFixtures()...)
		var out ExplainApplicationOutput
		res := h.call("explain_application", map[string]any{"name": "web", "namespace": "team-a"}, &out)
		Expect(res.IsError).To(BeFalse())

		Expect(out.Components).To(HaveLen(2))
		web := out.Components[0]
		Expect(web.DefinitionFound).To(BeTrue())
		Expect(web.Description).To(ContainSubstring("long-running"))
		Expect(web.Traits).To(ConsistOf("scaler"))
		Expect(web.Status).To(HaveLen(1))
		Expect(web.Status[0].Cluster).To(Equal("local"))
		Expect(web.Resources).To(HaveLen(2), "Deployment and Secret")
		var deploy ResourceSummary
		for _, r := range web.Resources {
			if r.Kind == "Deployment" {
				deploy = r
			}
		}
		Expect(deploy.Health.Status).To(Equal("Healthy"))
		Expect(deploy.Pods).To(Equal(map[string]int{"Running": 2}))
		Expect(deploy.Restarts).To(Equal(4))

		db := out.Components[1]
		Expect(db.DefinitionFound).To(BeFalse())
		Expect(db.DependsOn).To(ConsistOf("web"))
		Expect(db.Resources).To(BeEmpty())

		Expect(out.Workflow.SuspendedSteps).To(ConsistOf("approve"))
		Expect(out.Events).To(HaveLen(2), "the event on the unmanaged object is excluded")
		Expect(out.Events[0].Object).To(Equal("Pod/web-abc-2"))
		Expect(out.Events[0].Message).To(Equal("Back-off restarting failed container web; token=" + redactedValue))
		Expect(out.Events[1].Object).To(Equal("Application/web"))

		messages := []string{}
		for _, f := range out.Findings {
			messages = append(messages, f.Severity+": "+f.Message)
			Expect(f.Evidence).NotTo(BeEmpty(), f.Message)
		}
		Expect(messages).To(ContainElements(
			ContainSubstring(`error: component "db" uses type "helm" which is not published`),
			ContainSubstring("warning: status describes generation 2 but the spec is at generation 3"),
			ContainSubstring(`warning: component "web": pods under Deployment web have restarted 4 times`),
			ContainSubstring("warning: Pod/web-abc-2: BackOff"),
			ContainSubstring("info: workflow is suspended at approve; next action: vela workflow resume web -n team-a"),
			ContainSubstring("info: application phase is workflowSuspending"),
		))
		for i := 1; i < len(out.Findings); i++ {
			rank := map[string]int{SeverityError: 0, SeverityWarning: 1, SeverityInfo: 2}
			Expect(rank[out.Findings[i-1].Severity]).To(BeNumerically("<=", rank[out.Findings[i].Severity]))
		}
		Expect(out.Unknowns).To(ContainElement(ContainSubstring("observability MCP")))
		Expect(out.Evidence).To(ContainElements(
			ContainSubstring("Application team-a/web generation 3"),
			ContainSubstring("ComponentDefinition vela-system/webservice"),
			ContainSubstring("Event objects in namespace team-a"),
		))
	})

	It("explains a failed application without managed resources", func() {
		h = startHarness(Options{}, explainAppFixtures()...)
		var out ExplainApplicationOutput
		h.call("explain_application", map[string]any{"name": "batch", "namespace": "team-b"}, &out)
		Expect(out.Healthy).To(BeFalse())
		Expect(out.Components).To(HaveLen(1))
		Expect(out.Components[0].Resources).To(BeEmpty())
		Expect(out.Findings[0].Severity).To(Equal(SeverityError))
		messages := []string{}
		for _, f := range out.Findings {
			messages = append(messages, f.Message)
		}
		Expect(messages).To(ContainElements(
			ContainSubstring("application phase is workflowFailed"),
			ContainSubstring(`workflow step "run" (deploy) failed: Timeout`),
			ContainSubstring("no component status has been recorded yet"),
		))
	})
})
