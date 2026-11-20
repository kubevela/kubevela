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

package appfile

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/inherit"
)

// levelNameParent sets the image from a required parameter, so a child that
// writes another image is only refused once a render binds it.
const levelNameParent = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: template: spec: containers: [{name: "main", image: parameter.image}]
}
parameter: image: string
`

const levelNameChild = `
parameter: image: *"nginx" | string
$super: properties: {image: parameter.image}
output: spec: template: spec: containers: [{image: "busybox"}]
`

var _ = Describe("a render-time refusal from an inheritance chain", func() {
	render := func(component, typ string) error {
		engine := newEngineFor(types.TypeComponentDefinition, component, typ,
			inherit.Level{Name: "platform-web", Template: levelNameParent})
		ctx := process.NewContext(process.ContextData{AppName: "shop", CompName: component, Namespace: "default"})
		return engine.Complete(ctx, levelNameChild, map[string]interface{}{})
	}

	It("names the component and the definition it uses", func() {
		Expect(render("checkout", "checkout-web")).To(MatchError(ContainSubstring(
			"component checkout (type checkout-web) sets output.spec.template.spec.containers[0].image, which platform-web already sets")))
	})

	It("names it once when the component is called after its definition", func() {
		err := render("checkout-web", "checkout-web")
		Expect(err).To(MatchError(ContainSubstring("component checkout-web sets output.spec.template.spec.containers[0].image")))
		Expect(err.Error()).NotTo(ContainSubstring("(type "))
	})
})
