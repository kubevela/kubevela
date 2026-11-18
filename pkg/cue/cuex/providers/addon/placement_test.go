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

package addon

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	v1alpha1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/addon/service/api"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/validation"
	"github.com/oam-dev/kubevela/pkg/features"
)

// The provider hands a valid placement to the renderer and refuses a bad one
// with the same message as the module provider, before the renderer is
// reached. It runs under a validation-only context too, so admission refuses
// the same request.
var _ = Describe("Render placement", func() {
	var fake *fakeRenderer

	BeforeEach(func() {
		featuregatetesting.SetFeatureGateDuringTest(GinkgoTB(), utilfeature.DefaultMutableFeatureGate, features.EnableAddonComponent, true)
		prev := api.DefaultRenderer()
		DeferCleanup(func() { api.SetDefaultRenderer(prev) })
		fake = &fakeRenderer{res: &api.AddonResult{Application: map[string]interface{}{}}}
		api.SetDefaultRenderer(fake)
	})

	It("passes a valid placement to the renderer", func() {
		place := &v1alpha1.Placement{ClusterLabelSelector: map[string]string{"tier": "runtime"}}
		_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Addon: "platform", Placement: place}})
		Expect(err).NotTo(HaveOccurred())
		Expect(fake.req.Placement).To(Equal(place))
	})

	DescribeTable("refuses a bad placement before the renderer",
		func(ctx context.Context) {
			_, err := Render(ctx, &RenderParams{Params: RenderVars{Addon: "platform", Placement: &v1alpha1.Placement{
				Clusters: []string{"spoke"}, ClusterLabelSelector: map[string]string{"tier": "runtime"},
			}}})
			Expect(err).To(MatchError("placement must set clusters or clusterLabelSelector, not both"))
			Expect(fake.req).To(Equal(api.AddonRequest{}), "the renderer must not be reached")
		},
		Entry("at render", context.Background()),
		Entry("at admission", validation.WithValidationOnly(context.Background())),
	)
})
