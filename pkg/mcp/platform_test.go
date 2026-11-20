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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("platform information", func() {
	DescribeTable("parses the controller's feature gates",
		func(command, args []string, want map[string]bool) {
			d := &appsv1.Deployment{Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "gateway", Command: command, Args: args}},
			}}}}
			Expect(parseFeatureGates(d)).To(Equal(want))
		},
		Entry("one flag per gate", nil, []string{"--feature-gates=A=true", "--feature-gates=B=false"}, map[string]bool{"A": true, "B": false}),
		Entry("the flag and its value as separate arguments", nil, []string{"--feature-gates", "ClientIdentityPenetration=true,B=false"},
			map[string]bool{"ClientIdentityPenetration": true, "B": false}),
		Entry("a trailing flag with no value", nil, []string{"--feature-gates"}, nil),
		Entry("Args override Command, as the binary reads Command then Args",
			[]string{"/gateway", "--feature-gates=ClientIdentityPenetration=false"}, []string{"--feature-gates=ClientIdentityPenetration=true"},
			map[string]bool{"ClientIdentityPenetration": true}),
	)

	It("labels get_platform_info mixed, dated by the cache read, when its platform fields come from the cache", func() {
		h := startHarness(Options{}, platformFixtures()...)
		DeferCleanup(h.close)
		var first GetPlatformInfoOutput
		Expect(h.call("get_platform_info", nil, &first).IsError).To(BeFalse())
		Expect(first.Meta.Source).To(Equal(SourceLive))
		time.Sleep(20 * time.Millisecond)
		secondStart := time.Now()
		var second GetPlatformInfoOutput
		Expect(h.call("get_platform_info", nil, &second).IsError).To(BeFalse())
		Expect(second.Meta.Source).To(Equal(SourceMixed), "addons and the rest are still read live")
		Expect(second.Meta.Warnings).To(ContainElement(SatisfyAll(
			ContainSubstring("cached platform read at "+first.Meta.ObservedAt.Format(time.RFC3339)),
			ContainSubstring("; addons, cloudProvisioners were read live by this call"))),
			"a call without service or namespace reads neither provisioner nor practice")
		Expect(second.Meta.ObservedAt.After(first.Meta.ObservedAt)).To(BeFalse(),
			"the cached snapshot is dated when it was read, not when it was served")
		Expect(first.Meta.ObservedAt).To(BeTemporally("<=", secondStart.Add(-20*time.Millisecond)))
		var defs ListDefinitionsOutput
		h.call("list_definitions", nil, &defs)
		Expect(defs.Meta.Source).To(Equal(SourceLive), "other tools read their payload live")
	})
})
