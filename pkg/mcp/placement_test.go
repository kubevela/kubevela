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
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("placement of a topology policy", func() {
	topology := func(properties string) placement {
		app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Policies: []v1beta1.AppPolicy{{
			Name: "where", Type: "topology", Properties: &runtime.RawExtension{Raw: []byte(properties)},
		}}}}
		return placementOf(app)
	}

	// The controller reads clusters, then clusterLabelSelector, and the
	// deprecated clusterSelector only when neither is set. A null value is the
	// same as an absent key.
	DescribeTable("follows the selector the controller would read",
		func(properties string, selectsClusters, unknown bool) {
			p := topology(properties)
			Expect(p.clusters != nil).To(Equal(selectsClusters), "selects clusters")
			Expect(p.unknown != nil).To(Equal(unknown), "placement unknown")
		},
		Entry("clusters by name", `{"clusters":["local"]}`, true, false),
		Entry("clusters by label", `{"clusterLabelSelector":{"region":"eu"}}`, true, false),
		Entry("the deprecated selector alone", `{"clusterSelector":{"region":"eu"}}`, false, true),
		Entry("the deprecated selector set to null", `{"clusterSelector":null}`, false, false),
		Entry("the deprecated selector beside clusters", `{"clusters":["local"],"clusterSelector":{"region":"eu"}}`, true, false),
		Entry("the deprecated selector beside a label selector", `{"clusterLabelSelector":{"region":"eu"},"clusterSelector":{"region":"us"}}`, true, false),
		Entry("the deprecated selector beside null modern selectors", `{"clusters":null,"clusterLabelSelector":null,"clusterSelector":{"region":"eu"}}`, false, true),
		Entry("a namespace only", `{"namespace":"team-a"}`, false, false),
	)
})

var _ = Describe("an event and the condition transition it belongs to", func() {
	transition := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	at := func(offset time.Duration) *time.Time {
		t := transition.Add(offset)
		return &t
	}

	DescribeTable("beforeTransition",
		func(last, transitionAt *time.Time, want bool) {
			Expect(beforeTransition(last, transitionAt)).To(Equal(want))
		},
		Entry("an event after the transition is kept", at(time.Minute), &transition, false),
		Entry("an event written just before the condition is kept", at(-10*time.Second), &transition, false),
		Entry("an event from an earlier failure is dropped", at(-20*time.Minute), &transition, true),
		Entry("an event with no time is kept", nil, &transition, false),
		Entry("a condition with no transition time keeps every event", at(-20*time.Minute), nil, false),
	)
})

var _ = Describe("the cache warning of get_platform_info", func() {
	readAt := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)

	DescribeTable("names only what the call read live",
		func(live []string, want string) {
			warning := cacheWarning(readAt, live)
			Expect(warning).To(ContainSubstring("cached platform read at 2026-11-15T09:00:00Z"))
			Expect(warning).To(HaveSuffix(want))
		},
		Entry("nothing, when no client could read", nil, "; this call read nothing else"),
		Entry("one field", []string{"addons"}, "; addons was read live by this call"),
		Entry("several fields", []string{"addons", "cloudProvisioners", "provisioner"}, "; addons, cloudProvisioners, provisioner were read live by this call"),
	)
})
