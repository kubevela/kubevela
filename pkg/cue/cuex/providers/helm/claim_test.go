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

package helm

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("claimPatch", func() {
	velaCtx := &ContextParams{AppName: "shop", AppNamespace: "team-a", Name: "redis"}
	ours := func() map[string]string {
		return map[string]string{"app.oam.dev/name": "shop", "app.oam.dev/namespace": "team-a", "app.oam.dev/component": "redis"}
	}

	It("marks an object this component dispatched for the release", func() {
		patch, ok := claimPatch(ours(), nil, velaCtx, "redis", "team-a")
		Expect(ok).To(BeTrue())
		var got map[string]map[string]map[string]string
		Expect(json.Unmarshal(patch, &got)).To(Succeed())
		Expect(got["metadata"]["labels"]).To(Equal(map[string]string{"app.kubernetes.io/managed-by": "Helm"}))
		Expect(got["metadata"]["annotations"]).To(Equal(map[string]string{
			"meta.helm.sh/release-name": "redis", "meta.helm.sh/release-namespace": "team-a",
		}))
	})

	It("leaves an object already marked for the release", func() {
		labels := ours()
		labels["app.kubernetes.io/managed-by"] = "Helm"
		_, ok := claimPatch(labels, map[string]string{
			"meta.helm.sh/release-name": "redis", "meta.helm.sh/release-namespace": "team-a",
		}, velaCtx, "redis", "team-a")
		Expect(ok).To(BeFalse())
	})

	It("marks an object whose Helm annotations are empty, as if unset", func() {
		_, ok := claimPatch(ours(), map[string]string{"meta.helm.sh/release-name": "", "meta.helm.sh/release-namespace": ""}, velaCtx, "redis", "team-a")
		Expect(ok).To(BeTrue())
	})

	It("leaves an object marked for another release", func() {
		_, ok := claimPatch(ours(), map[string]string{"meta.helm.sh/release-name": "other"}, velaCtx, "redis", "team-a")
		Expect(ok).To(BeFalse())
		_, ok = claimPatch(ours(), map[string]string{"meta.helm.sh/release-namespace": "elsewhere"}, velaCtx, "redis", "team-a")
		Expect(ok).To(BeFalse())
	})

	It("leaves objects of another component, Application or namespace", func() {
		for _, k := range []string{"app.oam.dev/name", "app.oam.dev/namespace", "app.oam.dev/component"} {
			labels := ours()
			labels[k] = "someone-else"
			_, ok := claimPatch(labels, nil, velaCtx, "redis", "team-a")
			Expect(ok).To(BeFalse(), k)
		}
		_, ok := claimPatch(nil, nil, velaCtx, "redis", "team-a")
		Expect(ok).To(BeFalse())
	})

	It("claims nothing without a KubeVela context", func() {
		_, ok := claimPatch(ours(), nil, nil, "redis", "team-a")
		Expect(ok).To(BeFalse())
		// An unlabelled object matches a context missing any of the three, so none is claimed.
		for _, partial := range []*ContextParams{
			{AppNamespace: "team-a", Name: "cache"}, {AppName: "shop", Name: "cache"}, {AppName: "shop", AppNamespace: "team-a"},
		} {
			_, ok = claimPatch(map[string]string{"app.oam.dev/name": partial.AppName, "app.oam.dev/namespace": partial.AppNamespace, "app.oam.dev/component": partial.Name}, nil, partial, "redis", "team-a")
			Expect(ok).To(BeFalse(), "%+v", *partial)
		}
	})
})
