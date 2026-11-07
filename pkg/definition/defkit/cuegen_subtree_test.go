/*
Copyright 2025 The KubeVela Authors.

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

package defkit_test

import (
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

var _ = Describe("Parent and child writes on the same subtree", func() {
	// an ergonomic field guarded on the passthrough being absent, next to a
	// whole-object passthrough guarded on it being present
	component := func() *defkit.ComponentDefinition {
		size := defkit.String("storageSize").Default("1Gi")
		adv := defkit.Map("storage").Optional()
		return defkit.NewComponent("mixed-paths").
			Description("x").
			Workload("example.com/v1", "Thing").
			Params(size, adv).
			Template(func(tpl *defkit.Template) {
				size := defkit.String("storageSize")
				adv := defkit.Map("storage")
				tpl.Output(defkit.NewResource("example.com/v1", "Thing").
					SetIf(adv.NotSet(), "spec.storage.size", size).
					SetIf(adv.IsSet(), "spec.storage", adv))
			})
	}

	extractTemplateBody := func(full string) string {
		idx := strings.Index(full, "template: {")
		Expect(idx).To(BeNumerically(">=", 0))
		body := full[idx+len("template: {"):]
		j := strings.LastIndex(body, "}")
		Expect(j).To(BeNumerically(">=", 0))
		return body[:j]
	}

	lookupString := func(src, path string) (string, error) {
		v := cuecontext.New().CompileString(src)
		if err := v.Err(); err != nil {
			return "", err
		}
		return v.LookupPath(cue.ParsePath(path)).String()
	}

	It("writes the passthrough object when the parent parameter is set", func() {
		tmpl := extractTemplateBody(component().ToCue())
		got, err := lookupString(
			"parameter: {storageSize: \"1Gi\", storage: {size: \"5Gi\"}}\n"+tmpl,
			"output.spec.storage.size")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("5Gi"), "the user's passthrough value must survive")
	})

	It("emits a mixed node's directives exactly once", func() {
		size := defkit.String("storageSize").Default("1Gi")
		adv := defkit.Map("storage").Optional()
		comp := defkit.NewComponent("directive-once").
			Description("x").
			Workload("example.com/v1", "Thing").
			Params(size, adv).
			Template(func(tpl *defkit.Template) {
				size := defkit.String("storageSize")
				adv := defkit.Map("storage")
				// unconditional child: the recursive clone cannot decompose,
				// so it reaches the directive loop a second time
				tpl.Output(defkit.NewResource("example.com/v1", "Thing").
					Set("spec.storage.size", size).
					SetIf(adv.IsSet(), "spec.storage", adv).
					Directive("spec.storage", "patchKey=name"))
			})
		out := comp.ToCue()
		Expect(strings.Count(out, "// +patchKey=name")).To(Equal(1), out)
	})

	It("writes the ergonomic field when the parent parameter is not set", func() {
		tmpl := extractTemplateBody(component().ToCue())
		got, err := lookupString(
			"parameter: {storageSize: \"1Gi\"}\n"+tmpl,
			"output.spec.storage.size")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("1Gi"), "the ergonomic default must survive")
	})
})
