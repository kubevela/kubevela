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

package service

import (
	"encoding/json"

	wfv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/addon/service/api"
)

func rawJSON(v interface{}) *runtime.RawExtension {
	b, err := json.Marshal(v)
	Expect(err).NotTo(HaveOccurred())
	return &runtime.RawExtension{Raw: b}
}

func moduleComponent(name string, props map[string]interface{}) common2.ApplicationComponent {
	return common2.ApplicationComponent{Name: name, Type: "module", Properties: rawJSON(props)}
}

func topologyPolicy(name string, clusters ...string) v1beta1.AppPolicy {
	return v1beta1.AppPolicy{Name: name, Type: v1alpha1.TopologyPolicyType, Properties: rawJSON(map[string]interface{}{"clusters": clusters})}
}

var _ = Describe("applyAddonPlacement", func() {
	It("adds a topology built from the placement", func() {
		app := &v1beta1.Application{}
		place := &v1alpha1.Placement{ClusterLabelSelector: map[string]string{"tier": "runtime"}, AllowEmpty: true}

		Expect(applyAddonPlacement(app, "platform", place, nil)).To(Succeed())

		Expect(app.Spec.Policies).To(HaveLen(1))
		Expect(app.Spec.Policies[0].Name).To(Equal("addon-placement"))
		Expect(app.Spec.Policies[0].Type).To(Equal(v1alpha1.TopologyPolicyType))
		Expect(string(app.Spec.Policies[0].Properties.Raw)).To(MatchJSON(`{"clusterLabelSelector":{"tier":"runtime"},"allowEmpty":true}`))
	})

	It("gives imports without a placement of their own the addon's placement", func() {
		app := &v1beta1.Application{}
		comps := []common2.ApplicationComponent{
			moduleComponent("store", map[string]interface{}{"module": "store"}),
			moduleComponent("cache", map[string]interface{}{"module": "cache", "placement": map[string]interface{}{"clusters": []interface{}{"spoke-c"}}}),
		}

		Expect(applyAddonPlacement(app, "platform", &v1alpha1.Placement{Clusters: []string{"spoke-a", "spoke-b"}}, comps)).To(Succeed())

		Expect(string(comps[0].Properties.Raw)).To(MatchJSON(`{"module":"store","placement":{"clusters":["spoke-a","spoke-b"]}}`),
			"an import that sets no placement follows the addon")
		Expect(string(comps[1].Properties.Raw)).To(MatchJSON(`{"module":"cache","placement":{"clusters":["spoke-c"]}}`),
			"an import's own placement is kept")
	})

	It("refuses an addon that places itself", func() {
		app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Policies: []v1beta1.AppPolicy{topologyPolicy("deploy-fluxcd", "spoke-a")}}}

		err := applyAddonPlacement(app, "fluxcd", &v1alpha1.Placement{Clusters: []string{"spoke-b"}}, nil)

		Expect(err).To(MatchError(`addon "fluxcd" places itself with its topology policy "deploy-fluxcd", so placement cannot be set; use the addon's own parameters instead`))
		Expect(app.Spec.Policies).To(HaveLen(1), "nothing is added when the request is refused")
	})

	It("refuses an addon that writes its own workflow", func() {
		app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Workflow: &v1beta1.Workflow{Steps: []wfv1alpha1.WorkflowStep{
			{WorkflowStepBase: wfv1alpha1.WorkflowStepBase{Name: "apply", Type: "apply-component"}},
		}}}}

		err := applyAddonPlacement(app, "custom", &v1alpha1.Placement{Clusters: []string{"spoke-b"}}, nil)

		Expect(err).To(MatchError(`addon "custom" defines its own workflow, so placement cannot be set; use the addon's own parameters instead`))
		Expect(app.Spec.Policies).To(BeEmpty(), "nothing is added when the request is refused")
	})

	It("does not reuse a policy name the addon already has", func() {
		app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Policies: []v1beta1.AppPolicy{
			{Name: "addon-placement", Type: v1alpha1.OverridePolicyType},
		}}}

		Expect(applyAddonPlacement(app, "platform", &v1alpha1.Placement{Clusters: []string{"spoke-a"}}, nil)).To(Succeed())

		Expect(app.Spec.Policies).To(HaveLen(2))
		Expect(app.Spec.Policies[1].Name).To(Equal("addon-placement-2"))
	})

	It("changes nothing without a request", func() {
		app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Policies: []v1beta1.AppPolicy{topologyPolicy("deploy-fluxcd", "spoke-a")}}}
		comps := []common2.ApplicationComponent{moduleComponent("store", map[string]interface{}{"module": "store"})}
		snapshot := func() string {
			b, err := json.Marshal(struct {
				App   *v1beta1.Application
				Comps []common2.ApplicationComponent
			}{app, comps})
			Expect(err).NotTo(HaveOccurred())
			return string(b)
		}
		before := snapshot()

		Expect(applyAddonPlacement(app, "fluxcd", nil, comps)).To(Succeed())

		Expect(snapshot()).To(MatchJSON(before))
	})
})

// Two requests that differ only in placement never share a cached render: the
// placement adds a topology to the rendered addon Application and is copied
// onto its imported modules.
var _ = Describe("cacheKey and placement", func() {
	It("gives each placement its own key", func() {
		base := api.AddonRequest{Name: "platform", Version: "1.0.0", Registry: "addons"}
		keys := map[string]string{}
		for name, place := range map[string]*v1alpha1.Placement{
			"none":        nil,
			"clusters":    {Clusters: []string{"spoke-a"}},
			"other":       {Clusters: []string{"spoke-b"}},
			"selector":    {ClusterLabelSelector: map[string]string{"tier": "runtime"}},
			"allow-empty": {ClusterLabelSelector: map[string]string{"tier": "runtime"}, AllowEmpty: true},
		} {
			req := base
			req.Placement = place
			key, ok := cacheKey(req)
			Expect(ok).To(BeTrue(), name)
			for other, k := range keys {
				Expect(key).NotTo(Equal(k), "%s and %s share a cache key", name, other)
			}
			keys[name] = key
		}
	})
})
