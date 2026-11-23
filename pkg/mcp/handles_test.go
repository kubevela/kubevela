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
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
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
		// Rendered pods carry only the name and component labels; the
		// namespace label is on the workload object, not its pods, so
		// Namespace scopes the selector instead.
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
		// The tracker also records a pod the Application owns in team-b.
		objs := explainAppFixtures()
		for _, o := range objs {
			if rt, ok := o.(*v1beta1.ResourceTracker); ok {
				rt.Spec.ManagedResources = append(rt.Spec.ManagedResources, v1beta1.ManagedResource{
					ClusterObjectReference: oamcommon.ClusterObjectReference{ObjectReference: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "team-b", Name: "spy"}},
					OAMObjectReference:     oamcommon.OAMObjectReference{Component: "web"},
				})
			}
		}
		objs = append(objs, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "spy", Namespace: "team-b"}})
		names := func(pods []PodHandle) []string {
			out := []string{}
			for _, p := range pods {
				out = append(out, p.Namespace+"/"+p.Name)
			}
			return out
		}

		h = startHarness(Options{}, objs...)
		Expect(names(explain("web", "team-a").Handles.Pods)).To(ContainElement("team-b/spy"), "unrestricted, the foreign pod is a handle")
		h.close()

		h = startHarness(Options{Namespaces: []string{"team-a"}}, objs...)
		pods := names(explain("web", "team-a").Handles.Pods)
		Expect(pods).NotTo(BeEmpty())
		Expect(pods).NotTo(ContainElement("team-b/spy"))
		for _, p := range pods {
			Expect(p).To(HavePrefix("team-a/"))
		}
	})

	It("uses the workflow start when the workflow runs a valuesFrom or publishVersion revision", func() {
		started := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
		for _, tc := range []struct{ appRevision, publishVersion string }{
			{appRevision: "web-v2-vf-1a2b3c4d"},
			{appRevision: "release-7", publishVersion: "release-7"},
		} {
			objs := explainAppFixtures()
			for _, o := range objs {
				if app, ok := o.(*v1beta1.Application); ok && app.Name == "web" {
					app.Status.Workflow.AppRevision = tc.appRevision
					if tc.publishVersion != "" {
						app.Annotations = map[string]string{oam.AnnotationPublishVersion: tc.publishVersion}
					}
				}
			}
			h = startHarness(Options{}, objs...)
			hd := explain("web", "team-a").Handles
			Expect(hd.AppliedAt).NotTo(BeNil(), tc.appRevision)
			Expect(*hd.AppliedAt).To(BeTemporally("==", started), tc.appRevision)
			Expect(hd.Window.Basis).To(Equal("the workflow start of revision "+tc.appRevision), tc.appRevision)
			h.close()
			h = nil
		}
	})

	It("names no revision in the basis when the workflow records none", func() {
		objs := explainAppFixtures()
		for _, o := range objs {
			if app, ok := o.(*v1beta1.Application); ok && app.Name == "web" {
				app.Status.Workflow.AppRevision = ""
			}
		}
		h = startHarness(Options{}, objs...)
		Expect(explain("web", "team-a").Handles.Window.Basis).To(Equal("the workflow start"))
	})

	Context("with more pods than the handles list", func() {
		manyPods := func() []client.Object {
			objs := explainAppFixtures()
			var template *corev1.Pod
			for _, o := range objs {
				if p, ok := o.(*corev1.Pod); ok && p.Name == "web-abc-1" {
					template = p
				}
			}
			for i := 0; i < maxHandlePods; i++ {
				p := template.DeepCopy()
				p.Name = fmt.Sprintf("web-extra-%02d", i)
				p.UID = apitypes.UID("pod-" + p.Name)
				objs = append(objs, p)
			}
			return objs
		}

		It("marks explain_application and get_application truncated", func() {
			h = startHarness(Options{}, manyPods()...)
			out := explain("web", "team-a")
			Expect(out.Handles.Pods).To(HaveLen(maxHandlePods))
			Expect(out.Unknowns).To(ContainElement(ContainSubstring("handles list the first")))
			Expect(out.Meta.Truncated).To(BeTrue())

			var got GetApplicationOutput
			res := h.call("get_application", map[string]any{"name": "web", "namespace": "team-a"}, &got)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(got.Handles.Pods).To(HaveLen(maxHandlePods))
			Expect(got.Meta.Truncated).To(BeTrue())
		})

		It("does not tell get_logs callers that handles list the first 50 pods", func() {
			logs := func(context.Context, *rest.Config, string, string, string, *corev1.PodLogOptions) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader("ready\n")), nil
			}
			h = startHarness(Options{PodLogs: logs}, manyPods()...)
			var out GetLogsOutput
			res := h.call("get_logs", map[string]any{"name": "web", "namespace": "team-a"}, &out)
			Expect(res.IsError).To(BeFalse(), toJSON(res))
			Expect(out.Pods).To(HaveLen(maxLogPods))
			// The handles name exactly the pods whose logs were read, not the
			// first five of the Application's pods.
			read := make([]PodHandle, 0, len(out.Pods))
			for _, p := range out.Pods {
				read = append(read, PodHandle{Cluster: p.Cluster, Namespace: p.Namespace, Name: p.Pod, Component: p.Component})
			}
			Expect(out.Handles.Pods).To(ConsistOf(read))
			Expect(out.Meta.Warnings).To(ContainElement(ContainSubstring("only the first 5 were read")))
			Expect(out.Meta.Warnings).NotTo(ContainElement(ContainSubstring("handles list the first")))
		})
	})
})
