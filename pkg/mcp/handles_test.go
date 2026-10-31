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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("correlation handles", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	explain := func(name, ns string) ExplainApplicationOutput {
		var out ExplainApplicationOutput
		res := h.call("explain_application", map[string]any{"name": name, "namespace": ns}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		return out
	}

	It("returns the exact identifiers for downstream queries", func() {
		h = startHarness(Options{}, explainAppFixtures()...)
		out := explain("web", "team-a")
		hd := out.Handles
		Expect(hd.Application).To(Equal("web"))
		Expect(hd.Namespace).To(Equal("team-a"))
		Expect(hd.Clusters).To(Equal([]string{"local"}))
		Expect(hd.Revision).To(Equal("web-v2"))
		Expect(hd.Pods).To(ConsistOf(
			PodHandle{Cluster: "local", Namespace: "team-a", Name: "web-abc-1", Component: "web"},
			PodHandle{Cluster: "local", Namespace: "team-a", Name: "web-abc-2", Component: "web"},
		))
		Expect(hd.Labels).To(Equal(map[string]string{"app.oam.dev/name": "web"}))
		Expect(hd.LabelSelector).To(Equal("app.oam.dev/name=web"))
		Expect(hd.MetricLabels).To(Equal(map[string]string{"app_oam_dev_name": "web"}))
		Expect(hd.ComponentLabels).To(HaveKeyWithValue("db", map[string]string{"app.oam.dev/name": "web", "app.oam.dev/component": "db"}))

		started := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
		Expect(*hd.AppliedAt).To(BeTemporally("==", started))
		Expect(hd.Window.From).To(BeTemporally("==", started.Add(-windowLead)))
		Expect(hd.Window.To).To(BeTemporally("~", time.Now(), time.Minute))
		Expect(hd.Window.Basis).To(ContainSubstring("workflow start of revision web-v2"))
		Expect(out.Evidence).To(ContainElement(ContainSubstring("apply time from the workflow start")))
	})

	It("falls back to the revision's creation time when the workflow has not started", func() {
		created := time.Date(2026, 10, 12, 18, 30, 0, 0, time.UTC)
		objs := explainAppFixtures()
		for _, o := range objs {
			if app, ok := o.(*v1beta1.Application); ok && app.Name == "web" {
				app.Status.Workflow.StartTime = metav1.Time{}
			}
		}
		objs = append(objs, &v1beta1.ApplicationRevision{ObjectMeta: metav1.ObjectMeta{Name: "web-v2", Namespace: "team-a", CreationTimestamp: metav1.NewTime(created)}})
		h = startHarness(Options{}, objs...)
		hd := explain("web", "team-a").Handles
		Expect(*hd.AppliedAt).To(BeTemporally("==", created))
		Expect(hd.Window.Basis).To(ContainSubstring("creation of ApplicationRevision web-v2"))
	})

	It("says so and uses the last hour when the apply time is unknown", func() {
		h = startHarness(Options{}, explainAppFixtures()...)
		out := explain("batch", "team-b")
		Expect(out.Handles.AppliedAt).To(BeNil())
		Expect(out.Handles.Window.From).To(BeTemporally("~", time.Now().Add(-windowFallback), time.Minute))
		Expect(out.Handles.Pods).To(BeEmpty())
		Expect(out.Handles.Clusters).To(Equal([]string{"local"}))
		Expect(out.Unknowns).To(ContainElement(ContainSubstring("when the running revision was applied")))
	})

	It("lists only pods inside the namespace allowlist", func() {
		h = startHarness(Options{Namespaces: []string{"team-a"}}, explainAppFixtures()...)
		for _, p := range explain("web", "team-a").Handles.Pods {
			Expect(p.Namespace).To(Equal("team-a"))
		}
	})
})
