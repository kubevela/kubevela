/*
Copyright 2021 The KubeVela Authors.

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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
)

var _ = Describe("Test validate CUE schematic Appfile", func() {
	type SubTestCase struct {
		compDefTmpl   string
		traitDefTmpl1 string
		traitDefTmpl2 string
		wantErrMsg    string
	}

	DescribeTable("Test validate outputs name unique", func(tc SubTestCase) {
		Expect("").Should(BeEmpty())
		wl := &Workload{
			Name:               "myweb",
			Type:               "worker",
			CapabilityCategory: types.CUECategory,
			Traits: []*Trait{
				{
					Name:               "myscaler",
					CapabilityCategory: types.CUECategory,
					Template:           tc.traitDefTmpl1,
					engine:             definition.NewTraitAbstractEngine("myscaler", pd),
				},
				{
					Name:               "myingress",
					CapabilityCategory: types.CUECategory,
					Template:           tc.traitDefTmpl2,
					engine:             definition.NewTraitAbstractEngine("myingress", pd),
				},
			},
			FullTemplate: &Template{
				TemplateStr: tc.compDefTmpl,
			},
			engine: definition.NewWorkloadAbstractEngine("myweb", pd),
		}

		ctxData := GenerateContextDataFromAppFile(&Appfile{
			Name:            "myapp",
			Namespace:       "test-ns",
			AppRevisionName: "myapp-v1",
		}, wl.Name)
		pCtx, err := newValidationProcessContext(wl, ctxData)
		Expect(err).Should(BeNil())
		Eventually(func() string {
			for _, tr := range wl.Traits {
				if err := tr.EvalContext(pCtx); err != nil {
					return err.Error()
				}
			}
			return ""
		}).Should(ContainSubstring(tc.wantErrMsg))
	},
		Entry("Succeed", SubTestCase{
			compDefTmpl: `
			output: {
				apiVersion: "apps/v1" 
				kind: "Deployment"
				}
			outputs: mysvc: {
				apiVersion: "v1"
				kind: "Service"
			}
			`,
			traitDefTmpl1: `
			outputs: mysvc1: {
				apiVersion: "v1"
				kind: "Service"
			}
			`,
			traitDefTmpl2: `
			outputs: mysvc2: {
				apiVersion: "v1"
				kind: "Service"
			}
			`,
			wantErrMsg: "",
		}),
		Entry("CompDef and TraitDef have same outputs", SubTestCase{
			compDefTmpl: `
			output: {
				apiVersion: "apps/v1" 
				kind: "Deployment"
				}
			outputs: mysvc1: {
				apiVersion: "v1"
				kind: "Service"
			}
			`,
			traitDefTmpl1: `
			outputs: mysvc1: {
				apiVersion: "v1"
				kind: "Service"
			}
			`,
			traitDefTmpl2: `
			outputs: mysvc2: {
				apiVersion: "v1"
				kind: "Service"
			}
			`,
			wantErrMsg: `auxiliary "mysvc1" already exits`,
		}),
		Entry("TraitDefs have same outputs", SubTestCase{
			compDefTmpl: `
			output: {
				apiVersion: "apps/v1" 
				kind: "Deployment"
				}
			outputs: mysvc: {
				apiVersion: "v1"
				kind: "Service"
			}
			`,
			traitDefTmpl1: `
			outputs: mysvc1: {
				apiVersion: "v1"
				kind: "Service"
			}
			`,
			traitDefTmpl2: `
			outputs: mysvc1: {
				apiVersion: "v1"
				kind: "Service"
			}
			`,
			wantErrMsg: `auxiliary "mysvc1" already exits`,
		}),
	)
})
