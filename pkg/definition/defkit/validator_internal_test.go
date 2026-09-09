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

package defkit

import (
	"cuelang.org/go/cue/cuecontext"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

// unrenderableValue is a Value the generator has no case for, standing in for a
// future defkit Value someone adds without teaching valueToCUE about it. Value
// cannot be implemented outside this package, which is why this lives here.
type unrenderableValue struct{}

func (unrenderableValue) expr()  {}
func (unrenderableValue) value() {}

var _ = ginkgo.Describe("Validator message rendering", func() {
	ginkgo.It("should fail in the generated CUE when the message cannot be rendered", func() {
		gen := NewCUEGenerator()
		comp := NewComponent("test").
			Params(String("name")).
			Validators(
				ValidateValue(unrenderableValue{}).
					FailWhen(LocalField("name").Eq("")).
					WithName("_validateName"),
			)

		cue := gen.GenerateParameterSchema(comp)

		gomega.Expect(cue).To(gomega.ContainSubstring("let _message = _|_"))
		gomega.Expect(cue).To(gomega.ContainSubstring("defkit: validator message expression cannot be rendered"))

		// A validator with an unusable message must not quietly render to CUE
		// that compiles and never fires. Making it bottom is what turns that
		// into something the author sees.
		v := cuecontext.New().CompileString(cue)
		gomega.Expect(v.Err()).To(gomega.HaveOccurred())
	})

	// Checking the rendered message as a whole is not enough. Nested inside an
	// interpolation the "_" placeholder does not stand out: "region '\(_)' bad"
	// is an ordinary non-concrete string, so CUE leaves it alone and the
	// validator never fires. These are the cases that have to fail loudly.
	ginkgo.DescribeTable("should fail when an unrenderable value is nested in the message",
		func(message Value) {
			comp := NewComponent("test").
				Params(String("region")).
				Validators(
					ValidateValue(message).
						FailWhen(LocalField("region").Eq("x")).
						WithName("_v"),
				)

			cue := NewCUEGenerator().GenerateParameterSchema(comp)
			gomega.Expect(cue).To(gomega.ContainSubstring("let _message = _|_"))

			v := cuecontext.New().CompileString(cue + "\nparameter: region: \"x\"")
			gomega.Expect(v.Err()).To(gomega.HaveOccurred())
		},
		ginkgo.Entry("in an interpolation",
			Interpolation(Lit("region '"), unrenderableValue{}, Lit("' bad"))),
		ginkgo.Entry("in a plus expression",
			Plus(Lit("region "), unrenderableValue{})),
		ginkgo.Entry("nested two deep",
			Interpolation(Lit("a "), Plus(Lit("b"), unrenderableValue{}), Lit(" c"))),
	)

	ginkgo.It("should still render a message whose parts are all known", func() {
		comp := NewComponent("test").
			Params(String("region")).
			Validators(
				ValidateValue(Interpolation(Lit("region '"), LocalField("region"), Lit("' bad"))).
					FailWhen(LocalField("region").Eq("x")).
					WithName("_v"),
			)

		cue := NewCUEGenerator().GenerateParameterSchema(comp)
		gomega.Expect(cue).NotTo(gomega.ContainSubstring("_|_"))

		v := cuecontext.New().CompileString(cue + "\nparameter: region: \"x\"")
		gomega.Expect(v.Err()).To(gomega.HaveOccurred())
		gomega.Expect(v.Err().Error()).To(gomega.ContainSubstring("region 'x' bad"))
	})
})
