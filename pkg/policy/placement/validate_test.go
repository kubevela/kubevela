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

package placement

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1alpha1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
)

var _ = Describe("Validate", func() {
	DescribeTable("accepts",
		func(p *v1alpha1.Placement) {
			Expect(Validate(p)).To(Succeed())
		},
		Entry("no placement request", nil),
		Entry("a cluster list", &v1alpha1.Placement{Clusters: []string{"spoke"}}),
		Entry("a label selector", &v1alpha1.Placement{ClusterLabelSelector: map[string]string{"tier": "runtime"}}),
		Entry("an empty selector, which matches every cluster", &v1alpha1.Placement{ClusterLabelSelector: map[string]string{}}),
	)

	DescribeTable("rejects",
		func(p *v1alpha1.Placement, want string) {
			Expect(Validate(p)).To(MatchError(want))
		},
		Entry("neither field", &v1alpha1.Placement{}, "placement must set clusters or clusterLabelSelector"),
		Entry("only allowEmpty", &v1alpha1.Placement{AllowEmpty: true}, "placement must set clusters or clusterLabelSelector"),
		Entry("both fields", &v1alpha1.Placement{Clusters: []string{"spoke"}, ClusterLabelSelector: map[string]string{"tier": "runtime"}},
			"placement must set clusters or clusterLabelSelector, not both"),
		Entry("an empty cluster list", &v1alpha1.Placement{Clusters: []string{}}, "placement clusters must name at least one cluster"),
		Entry("a blank cluster name", &v1alpha1.Placement{Clusters: []string{"spoke", ""}}, "placement clusters must not contain an empty name"),
		Entry("a whitespace cluster name", &v1alpha1.Placement{Clusters: []string{"spoke", "  "}}, "placement clusters must not contain an empty name"),
	)
})
