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
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// crashLooping makes the named pods behave like a container that keeps
// exiting: not ready, waiting in CrashLoopBackOff, with a terminated
// previous instance.
func crashLooping(objs []client.Object, names ...string) []client.Object {
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	for _, o := range objs {
		pod, ok := o.(*corev1.Pod)
		if !ok || !want[pod.Name] {
			continue
		}
		pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}
		for i := range pod.Status.ContainerStatuses {
			cs := &pod.Status.ContainerStatuses[i]
			cs.Ready = false
			cs.RestartCount = 7
			cs.State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}
			cs.LastTerminationState = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}}
		}
	}
	return objs
}

var _ = Describe("health derived from pods", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	deploymentRoot := func(out GetManagedResourceOutput) ResourceNode {
		for _, r := range out.Resources {
			if r.Kind == "Deployment" {
				Expect(r.Tree).NotTo(BeEmpty())
				return r.Tree[0]
			}
		}
		Fail("no Deployment in the result")
		return ResourceNode{}
	}

	It("marks a Deployment unhealthy when all its pods are failing", func() {
		h = startHarness(Options{}, crashLooping(managedFixtures(), "web-abc-1", "web-abc-2")...)
		var out GetManagedResourceOutput
		h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a"}, &out)
		root := deploymentRoot(out)
		Expect(root.Health.Status).To(Equal("UnHealthy"))
		Expect(root.Health.Derived).To(BeTrue())
		Expect(root.Health.Reason).To(Equal("2 of 2 pods are not healthy"))
		Expect(root.Health.Message).To(ContainSubstring("own health check reported Healthy"))
	})

	It("marks it progressing when only some pods are failing", func() {
		h = startHarness(Options{}, crashLooping(managedFixtures(), "web-abc-2")...)
		var out GetManagedResourceOutput
		h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a"}, &out)
		root := deploymentRoot(out)
		Expect(root.Health.Status).To(Equal("Progressing"))
		Expect(root.Health.Reason).To(Equal("1 of 2 pods are not healthy"))
	})

	It("leaves a Deployment with healthy pods alone", func() {
		h = startHarness(Options{}, managedFixtures()...)
		var out GetManagedResourceOutput
		h.call("get_managed_resource", map[string]any{"name": "web", "namespace": "team-a"}, &out)
		root := deploymentRoot(out)
		Expect(root.Health.Status).To(Equal("Healthy"))
		Expect(root.Health.Derived).To(BeFalse())
	})

	It("surfaces the derived status as an error finding in explain_application", func() {
		h = startHarness(Options{}, crashLooping(managedFixtures(), "web-abc-1", "web-abc-2")...)
		var out ExplainApplicationOutput
		h.call("explain_application", map[string]any{"name": "web", "namespace": "team-a"}, &out)
		var found *Finding
		for i := range out.Findings {
			if out.Findings[i].Severity == SeverityError && out.Findings[i].Message == `component "web": Deployment web is UnHealthy` {
				found = &out.Findings[i]
			}
		}
		Expect(found).NotTo(BeNil(), toJSON(out.Findings))
		Expect(found.Observed).To(ContainSubstring("2 of 2 pods are not healthy"))
		for _, c := range out.Components {
			for _, r := range c.Resources {
				if r.Kind == "Deployment" {
					Expect(r.Health.Status).To(Equal("UnHealthy"), "the summary no longer reads Healthy")
				}
			}
		}
	})
})
