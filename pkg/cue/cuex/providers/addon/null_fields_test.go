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

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/pkg/addon/service/api"
	"github.com/oam-dev/kubevela/pkg/features"
)

// The renderer writes a null where the addon Application must lose a field an
// earlier render wrote. The CueX runtime fills the call's returns into the
// definition's value, and a Go nil would become an incomplete value there, so
// the component could not render at all.
var _ = Describe("Render null fields", func() {
	rendered := func() map[string]interface{} {
		return map[string]interface{}{
			"kind": "Application",
			"spec": map[string]interface{}{
				"components": []interface{}{map[string]interface{}{"name": "marker", "dependsOn": nil}},
				"workflow": map[string]interface{}{
					"ref":   nil,
					"steps": []interface{}{map[string]interface{}{"name": "marker"}},
				},
			},
		}
	}

	BeforeEach(func() {
		featuregatetesting.SetFeatureGateDuringTest(GinkgoTB(), utilfeature.DefaultMutableFeatureGate, features.EnableAddonComponent, true)
		prev := api.DefaultRenderer()
		DeferCleanup(func() { api.SetDefaultRenderer(prev) })
		api.SetDefaultRenderer(&fakeRenderer{res: &api.AddonResult{Application: rendered()}})
	})

	It("keeps them null through the CUE value the definition reads", func() {
		ret, err := Render(context.Background(), &RenderParams{Params: RenderVars{Addon: "platform"}})
		Expect(err).NotTo(HaveOccurred())

		value := cuecontext.New().CompileString(`$returns: {application: {...}, ...}`).FillPath(cue.ParsePath("$returns"), ret.Returns)
		Expect(value.Err()).NotTo(HaveOccurred())
		out := map[string]interface{}{}
		Expect(value.LookupPath(cue.ParsePath("$returns.application")).Decode(&out)).To(Succeed())
		Expect(out).To(Equal(rendered()))
	})

	It("does not change the Application the renderer returned", func() {
		fake := &fakeRenderer{res: &api.AddonResult{Application: rendered()}}
		api.SetDefaultRenderer(fake)
		_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Addon: "platform"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(fake.res.Application).To(Equal(rendered()), "the renderer may cache or reuse its result")
	})
})
