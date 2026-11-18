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

package module

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	v1alpha1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/validation"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/module/service/api"
)

// The provider refuses a placement that names neither or both of clusters and
// clusterLabelSelector, with a plain message, before the renderer is reached.
// The check runs under a validation-only context too, so admission rejects
// the same request.
var _ = Describe("Render placement check", func() {
	var fake *fakeRenderer

	BeforeEach(func() {
		featuregatetesting.SetFeatureGateDuringTest(GinkgoTB(), utilfeature.DefaultMutableFeatureGate, features.EnableModuleComponent, true)
		prev := api.DefaultRenderer()
		DeferCleanup(func() { api.SetDefaultRenderer(prev) })
		fake = &fakeRenderer{res: &api.ModuleResult{Application: map[string]interface{}{}}}
		api.SetDefaultRenderer(fake)
	})

	DescribeTable("rejects a bad placement",
		func(ctx context.Context, place *v1alpha1.Placement, want string) {
			_, err := Render(ctx, &RenderParams{Params: RenderVars{Module: "store", Registry: "modules", Placement: place}})
			Expect(err).To(MatchError(want))
			Expect(fake.req).To(Equal(api.ModuleRequest{}), "the renderer must not be reached")
		},
		Entry("neither field, at render", context.Background(), &v1alpha1.Placement{},
			"placement must set clusters or clusterLabelSelector"),
		Entry("neither field, at admission", validation.WithValidationOnly(context.Background()), &v1alpha1.Placement{},
			"placement must set clusters or clusterLabelSelector"),
		Entry("both fields, at render", context.Background(),
			&v1alpha1.Placement{Clusters: []string{"spoke"}, ClusterLabelSelector: map[string]string{"tier": "runtime"}},
			"placement must set clusters or clusterLabelSelector, not both"),
		Entry("both fields, at admission", validation.WithValidationOnly(context.Background()),
			&v1alpha1.Placement{Clusters: []string{"spoke"}, ClusterLabelSelector: map[string]string{"tier": "runtime"}},
			"placement must set clusters or clusterLabelSelector, not both"),
	)
})
