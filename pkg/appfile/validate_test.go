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
	"testing"

	"cuelang.org/go/cue"
	"github.com/stretchr/testify/assert"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/features"
)

func TestTrait_EvalContext_OutputNameUniqueness(t *testing.T) {
	type SubTestCase struct {
		name          string
		compDefTmpl   string
		traitDefTmpl1 string
		traitDefTmpl2 string
		wantErrMsg    string
	}

	testCases := []SubTestCase{
		{
			name: "Succeed",
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
		},
		{
			name: "CompDef and TraitDef have same outputs",
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
		},
		{
			name: "TraitDefs have same outputs",
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
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
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
			assert.NoError(t, err)

			var evalErr error
			for _, tr := range wl.Traits {
				if err := tr.EvalContext(pCtx); err != nil {
					evalErr = err
					break
				}
			}

			if tc.wantErrMsg != "" {
				assert.Error(t, evalErr)
				assert.Contains(t, evalErr.Error(), tc.wantErrMsg)
			} else {
				assert.NoError(t, evalErr)
			}
		})
	}
}

func TestParser_ValidateComponentParams(t *testing.T) {
	testCases := []struct {
		name     string
		compName string
		template string
		params   map[string]interface{}
		wantErr  string
	}{
		{
			name:     "valid params and template",
			compName: "valid",
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
		},
		{
			name:     "invalid CUE in template",
			compName: "invalid-cue",
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
		},
		{
			name:     "missing required parameter",
			compName: "missing-required",
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
		},
		{
			name:     "parameter constraint violation",
			compName: "constraint-violation",
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
		},
		{
			name:     "invalid parameter block",
			compName: "invalid-param-block",
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
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wl := &Component{
				Name:         tc.compName,
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
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestValidationHelpers(t *testing.T) {
	t.Run("renderTemplate", func(t *testing.T) {
		tmpl := "output: {}"
		expected := "output: {}\ncontext: _\nparameter: _\n"
		assert.Equal(t, expected, renderTemplate(tmpl))
	})

	t.Run("cueParamBlock", func(t *testing.T) {
		t.Run("should handle empty params", func(t *testing.T) {
			out, err := cueParamBlock(map[string]any{})
			assert.NoError(t, err)
			assert.Equal(t, "parameter: {}", out)
		})

		t.Run("should handle valid params", func(t *testing.T) {
			params := map[string]any{"key": "value"}
			out, err := cueParamBlock(params)
			assert.NoError(t, err)
			assert.Equal(t, `parameter: {"key":"value"}`, out)
		})

		t.Run("should return error for unmarshallable params", func(t *testing.T) {
			params := map[string]any{"key": make(chan int)}
			_, err := cueParamBlock(params)
			assert.Error(t, err)
		})
	})

	t.Run("filterMissing", func(t *testing.T) {
		t.Run("should filter missing keys", func(t *testing.T) {
			keys := []string{"a", "b.c", "d"}
			provided := map[string]any{
				"a": 1,
				"b": map[string]any{
					"c": 2,
				},
			}
			out, err := filterMissing(keys, provided)
			assert.NoError(t, err)
			assert.Equal(t, []string{"d"}, out)
		})

		t.Run("should handle no missing keys", func(t *testing.T) {
			keys := []string{"a"}
			provided := map[string]any{"a": 1}
			out, err := filterMissing(keys, provided)
			assert.NoError(t, err)
			assert.Empty(t, out)
		})
	})

	t.Run("requiredFields", func(t *testing.T) {
		t.Run("should identify required fields", func(t *testing.T) {
			cueStr := `
			parameter: {
				name: string
				age: int
				nested: {
					field1: string
					field2: bool
				}
			}
			`
			var r cue.Runtime
			inst, err := r.Compile("", cueStr)
			assert.NoError(t, err)
			val := inst.Value()
			paramVal := val.LookupPath(cue.ParsePath("parameter"))

			fields, err := requiredFields(paramVal)
			assert.NoError(t, err)
			assert.ElementsMatch(t, []string{"name", "age", "nested.field1", "nested.field2"}, fields)
		})

		t.Run("should ignore optional and default fields", func(t *testing.T) {
			cueStr := `
			parameter: {
				name: string
				age?: int
				location: string | *"unknown"
				nested: {
					field1: string
					field2?: bool
				}
			}
			`
			var r cue.Runtime
			inst, err := r.Compile("", cueStr)
			assert.NoError(t, err)
			val := inst.Value()
			paramVal := val.LookupPath(cue.ParsePath("parameter"))

			fields, err := requiredFields(paramVal)
			assert.NoError(t, err)
			assert.ElementsMatch(t, []string{"name", "nested.field1"}, fields)
		})
	})
}

func TestEnforceRequiredParams(t *testing.T) {
	var r cue.Runtime
	cueStr := `
		parameter: {
			image: string
			replicas: int
			port: int
			data: {
				key: string
				value: string
			}
		}
		`
	inst, err := r.Compile("", cueStr)
	assert.NoError(t, err)
	root := inst.Value()

	t.Run("should pass if all params are provided directly", func(t *testing.T) {
		params := map[string]any{
			"image":    "nginx",
			"replicas": 2,
			"port":     80,
			"data": map[string]any{
				"key":   "k",
				"value": "v",
			},
		}
		app := &Appfile{}
		err := enforceRequiredParams(root, params, app)
		assert.NoError(t, err)
	})

	t.Run("should fail if params are missing", func(t *testing.T) {
		params := map[string]any{
			"image": "nginx",
		}
		app := &Appfile{}
		err := enforceRequiredParams(root, params, app)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing parameters: replicas,port,data.key,data.value")
	})
}

func TestParser_ValidateCUESchematicAppfile(t *testing.T) {
	assert.NoError(t, utilfeature.DefaultMutableFeatureGate.Set(string(features.EnableCueValidation)+"=true"))
	t.Cleanup(func() {
		assert.NoError(t, utilfeature.DefaultMutableFeatureGate.Set(string(features.EnableCueValidation)+"=false"))
	})

	t.Run("should validate a valid CUE schematic appfile", func(t *testing.T) {
		appfile := &Appfile{
			Name:      "test-app",
			Namespace: "test-ns",
			ParsedComponents: []*Component{
				{
					Name:               "my-comp",
					Type:               "worker",
					CapabilityCategory: types.CUECategory,
					Params: map[string]any{
						"image": "nginx",
					},
					FullTemplate: &Template{
						TemplateStr: `
							parameter: {
								image: string
							}
							output: {
								apiVersion: "apps/v1"
								kind: "Deployment"
								spec: {
									template: {
										spec: {
											containers: [{
												name: "my-container"
												image: parameter.image
											}]
										}
									}
								}
							}
						`,
					},
					engine: definition.NewWorkloadAbstractEngine("my-comp"),
					Traits: []*Trait{
						{
							Name:               "my-trait",
							CapabilityCategory: types.CUECategory,
							Template: `
								parameter: {
								domain: string
							}
							patch: {}
						`,
							Params: map[string]any{
								"domain": "example.com",
							},
							engine: definition.NewTraitAbstractEngine("my-trait"),
						},
					},
				},
			},
		}

		p := &Parser{}
		err := p.ValidateCUESchematicAppfile(appfile)
		assert.NoError(t, err)
	})

	t.Run("should return error for invalid trait evaluation", func(t *testing.T) {
		appfile := &Appfile{
			Name:      "test-app",
			Namespace: "test-ns",
			ParsedComponents: []*Component{
				{
					Name:               "my-comp",
					Type:               "worker",
					CapabilityCategory: types.CUECategory,
					Params: map[string]any{
						"image": "nginx",
					},
					FullTemplate: &Template{
						TemplateStr: `
							parameter: {
								image: string
							}
							output: {
								apiVersion: "apps/v1"
								kind: "Deployment"
							}
						`,
					},
					engine: definition.NewWorkloadAbstractEngine("my-comp"),
					Traits: []*Trait{
						{
							Name:               "my-trait",
							CapabilityCategory: types.CUECategory,
							Template: `
								// invalid CUE template
								parameter: {
									domain: string
								}
							patch: {
									invalid: {
							}
						`,
							Params: map[string]any{
								"domain": "example.com",
							},
							engine: definition.NewTraitAbstractEngine("my-trait"),
						},
					},
				},
			},
		}

		p := &Parser{}
		err := p.ValidateCUESchematicAppfile(appfile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot evaluate trait \"my-trait\"")
	})

	t.Run("should return error for missing parameters", func(t *testing.T) {
		appfile := &Appfile{
			Name:      "test-app",
			Namespace: "test-ns",
			ParsedComponents: []*Component{
				{
					Name:               "my-comp",
					Type:               "worker",
					CapabilityCategory: types.CUECategory,
					Params:             map[string]any{}, // no params provided
					FullTemplate: &Template{
						TemplateStr: `
							parameter: {
								image: string
							}
							output: {
								apiVersion: "apps/v1"
								kind: "Deployment"
							}
						`,
					},
					engine: definition.NewWorkloadAbstractEngine("my-comp"),
				},
			},
		}

		p := &Parser{}
		err := p.ValidateCUESchematicAppfile(appfile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing parameters: image")
	})

	t.Run("should skip non-CUE components", func(t *testing.T) {
		appfile := &Appfile{
			Name:      "test-app",
			Namespace: "test-ns",
			ParsedComponents: []*Component{
				{
					Name:               "my-comp",
					Type:               "helm",
					CapabilityCategory: types.TerraformCategory,
				},
			},
		}
		p := &Parser{}
		err := p.ValidateCUESchematicAppfile(appfile)
		assert.NoError(t, err)
	})
}
