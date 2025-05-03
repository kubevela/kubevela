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
	. "github.com/onsi/ginkgo/v2"
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
		wl := &Component{
			Name:               "myweb",
			Type:               "worker",
			CapabilityCategory: types.CUECategory,
			Traits: []*Trait{
				{
					Name:               "myscaler",
					CapabilityCategory: types.CUECategory,
					Template:           tc.traitDefTmpl1,
					engine:             definition.NewTraitAbstractEngine("myscaler"),
				},
				{
					Name:               "myingress",
					CapabilityCategory: types.CUECategory,
					Template:           tc.traitDefTmpl2,
					engine:             definition.NewTraitAbstractEngine("myingress"),
				},
			},
			FullTemplate: &Template{
				TemplateStr: tc.compDefTmpl,
			},
			engine: definition.NewWorkloadAbstractEngine("myweb"),
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

var _ = Describe("Test ValidateComponentParams", func() {
	type ParamTestCase struct {
		name     string
		template string
		params   map[string]interface{}
		wantErr  string
	}

	DescribeTable("ValidateComponentParams cases", func(tc ParamTestCase) {
		wl := &Component{
			Name:         tc.name,
			Type:         "worker",
			FullTemplate: &Template{TemplateStr: tc.template},
			Params:       tc.params,
		}
		app := &Appfile{
			Name:      "myapp",
			Namespace: "test-ns",
		}
		ctxData := GenerateContextDataFromAppFile(app, wl.Name)
		parser := &Parser{}
		err := parser.ValidateComponentParams(ctxData, wl, app)
		if tc.wantErr == "" {
			Expect(err).To(BeNil())
		} else {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tc.wantErr))
		}
	},
		Entry("valid params and template", ParamTestCase{
			name: "valid",
			template: `
			parameter: {
				replicas: int | *1
			}
			output: {
				apiVersion: "apps/v1"
				kind: "Deployment"
			}
			`,
			params: map[string]interface{}{
				"replicas": 2,
			},
			wantErr: "",
		}),
		Entry("invalid CUE in template", ParamTestCase{
			name: "invalid-cue",
			template: `
			parameter: {
				replicas: int | *1
			}
			output: {
				apiVersion: "apps/v1"
				kind: "Deployment"
				invalidField: {
			}
			`,
			params: map[string]interface{}{
				"replicas": 2,
			},
			wantErr: "CUE compile error",
		}),
		Entry("missing required parameter", ParamTestCase{
			name: "missing-required",
			template: `
			parameter: {
				replicas: int
			}
			output: {
				apiVersion: "apps/v1"
				kind: "Deployment"
			}
			`,
			params:  map[string]interface{}{},
			wantErr: "component \"missing-required\": missing parameters: replicas",
		}),
		Entry("parameter constraint violation", ParamTestCase{
			name: "constraint-violation",
			template: `
			parameter: {
				replicas: int & >0
			}
			output: {
				apiVersion: "apps/v1"
				kind: "Deployment"
			}
			`,
			params: map[string]interface{}{
				"replicas": -1,
			},
			wantErr: "parameter constraint violation",
		}),
		Entry("invalid parameter block", ParamTestCase{
			name: "invalid-param-block",
			template: `
			parameter: {
				replicas: int | *1
			}
			output: {
				apiVersion: "apps/v1"
				kind: "Deployment"
			}
			`,
			params: map[string]interface{}{
				"replicas": "not-an-int",
			},
			wantErr: "parameter constraint violation",
		}),
	)
})
