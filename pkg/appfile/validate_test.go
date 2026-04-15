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
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
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

func TestCheckUndeclaredParams(t *testing.T) {
	var r cue.Runtime

	t.Run("undeclared top-level parameter", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { name: string }`)
		assert.NoError(t, err)
		schema := inst.Value().LookupPath(cue.ParsePath("parameter"))
		err = checkUndeclaredParams(schema, map[string]any{
			"name":            "test",
			"otherUndeclared": "bad",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "undeclared parameters")
		assert.Contains(t, err.Error(), "otherUndeclared")
	})

	t.Run("undeclared nested parameter", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { data: { key: string } }`)
		assert.NoError(t, err)
		schema := inst.Value().LookupPath(cue.ParsePath("parameter"))
		err = checkUndeclaredParams(schema, map[string]any{
			"data": map[string]any{
				"key":   "ok",
				"extra": "bad",
			},
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "data.extra")
	})

	t.Run("all valid parameters", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { name: string, count: int }`)
		assert.NoError(t, err)
		schema := inst.Value().LookupPath(cue.ParsePath("parameter"))
		err = checkUndeclaredParams(schema, map[string]any{
			"name":  "test",
			"count": 1,
		})
		assert.NoError(t, err)
	})

	t.Run("empty parameters", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { name: string }`)
		assert.NoError(t, err)
		schema := inst.Value().LookupPath(cue.ParsePath("parameter"))
		err = checkUndeclaredParams(schema, map[string]any{})
		assert.NoError(t, err)
	})

	t.Run("optional parameters omitted", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { name: string, label?: string }`)
		assert.NoError(t, err)
		schema := inst.Value().LookupPath(cue.ParsePath("parameter"))
		err = checkUndeclaredParams(schema, map[string]any{
			"name": "test",
		})
		assert.NoError(t, err)
	})

	t.Run("optional parameter provided is valid", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { name: string, label?: string }`)
		assert.NoError(t, err)
		schema := inst.Value().LookupPath(cue.ParsePath("parameter"))
		err = checkUndeclaredParams(schema, map[string]any{
			"name":  "test",
			"label": "ok",
		})
		assert.NoError(t, err)
	})

	t.Run("pattern constraint at top level allows any keys", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: [string]: string`)
		assert.NoError(t, err)
		schema := inst.Value().LookupPath(cue.ParsePath("parameter"))
		err = checkUndeclaredParams(schema, map[string]any{
			"anyKey":     "value1",
			"anotherKey": "value2",
		})
		assert.NoError(t, err)
	})

	t.Run("nested pattern constraint allows any keys", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { name: string, labels?: [string]: string }`)
		assert.NoError(t, err)
		schema := inst.Value().LookupPath(cue.ParsePath("parameter"))
		err = checkUndeclaredParams(schema, map[string]any{
			"name": "test",
			"labels": map[string]any{
				"app":     "myapp",
				"version": "v1",
			},
		})
		assert.NoError(t, err)
	})

	t.Run("webservice-like schema with labels and annotations", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: {
			image: string
			labels?: [string]: string
			annotations?: [string]: string
		}`)
		assert.NoError(t, err)
		schema := inst.Value().LookupPath(cue.ParsePath("parameter"))
		err = checkUndeclaredParams(schema, map[string]any{
			"image": "nginx",
			"labels": map[string]any{
				"app": "myapp",
			},
			"annotations": map[string]any{
				"note": "test",
			},
		})
		assert.NoError(t, err)
	})

	t.Run("undeclared fields sorted in error message", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { name: string }`)
		assert.NoError(t, err)
		schema := inst.Value().LookupPath(cue.ParsePath("parameter"))
		err = checkUndeclaredParams(schema, map[string]any{
			"name":  "test",
			"zebra": "z",
			"alpha": "a",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "alpha,zebra")
	})
}

func TestValidateUndeclaredParams_FeatureGate(t *testing.T) {
	prevCue := utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableCueValidation)
	prevUndeclared := utilfeature.DefaultMutableFeatureGate.Enabled(features.ValidateUndeclaredParameters)
	t.Cleanup(func() {
		assert.NoError(t, utilfeature.DefaultMutableFeatureGate.Set(
			fmt.Sprintf("%s=%t,%s=%t",
				string(features.EnableCueValidation), prevCue,
				string(features.ValidateUndeclaredParameters), prevUndeclared)))
	})

	t.Run("gate disabled - undeclared params allowed", func(t *testing.T) {
		assert.NoError(t, utilfeature.DefaultMutableFeatureGate.Set(
			string(features.EnableCueValidation)+"=true,"+
				string(features.ValidateUndeclaredParameters)+"=false"))

		wl := &Component{
			Name: "my-comp",
			Type: "worker",
			FullTemplate: &Template{TemplateStr: `
				parameter: { name: string }
				output: { apiVersion: "v1", kind: "ConfigMap" }
			`},
			Params: map[string]any{"name": "test", "extra": "bad"},
		}
		app := &Appfile{Name: "myapp", Namespace: "test-ns"}
		ctxData := GenerateContextDataFromAppFile(app, wl.Name)
		err := (&Parser{}).ValidateComponentParams(ctxData, wl, app)
		assert.NoError(t, err, "Should allow undeclared params when gate is disabled")
	})

	t.Run("gate enabled - undeclared params rejected", func(t *testing.T) {
		assert.NoError(t, utilfeature.DefaultMutableFeatureGate.Set(
			string(features.EnableCueValidation)+"=true,"+
				string(features.ValidateUndeclaredParameters)+"=true"))

		wl := &Component{
			Name: "my-comp",
			Type: "worker",
			FullTemplate: &Template{TemplateStr: `
				parameter: { name: string }
				output: { apiVersion: "v1", kind: "ConfigMap" }
			`},
			Params: map[string]any{"name": "test", "extra": "bad"},
		}
		app := &Appfile{Name: "myapp", Namespace: "test-ns"}
		ctxData := GenerateContextDataFromAppFile(app, wl.Name)
		err := (&Parser{}).ValidateComponentParams(ctxData, wl, app)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "undeclared parameters")
		assert.Contains(t, err.Error(), "extra")
	})
}

// TestValidateCUESchematicAppfile_WorkflowSuppliedParams tests validation with workflow-supplied parameters (issue #7022)
func TestValidateCUESchematicAppfile_WorkflowSuppliedParams(t *testing.T) {
	prevCue := utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableCueValidation)
	assert.NoError(t, utilfeature.DefaultMutableFeatureGate.Set(string(features.EnableCueValidation)+"=true"))
	t.Cleanup(func() {
		assert.NoError(t, utilfeature.DefaultMutableFeatureGate.Set(
			fmt.Sprintf("%s=%t", string(features.EnableCueValidation), prevCue)))
	})

	componentTemplate := `
		parameter: {
			image: string
			port: int | *80
		}
		output: {
			apiVersion: "apps/v1"
			kind: "Deployment"
			spec: {
				template: {
					spec: {
						containers: [{
							name: "main"
							image: parameter.image
							ports: [{
								containerPort: parameter.port
							}]
						}]
					}
				}
			}
		}
	`

	traitTemplate := `
		parameter: {
			key: string
			value: string
		}
		patch: {
			metadata: {
				labels: {
					(parameter.key): parameter.value
				}
			}
		}
	`

	t.Run("workflow supplies param - NO traits - should PASS", func(t *testing.T) {
		appfile := &Appfile{
			Name:      "test-app",
			Namespace: "test-ns",
			ParsedComponents: []*Component{
				{
					Name:               "my-webservice",
					Type:               "webservice",
					CapabilityCategory: types.CUECategory,
					Params: map[string]any{
						"port": 80,
					},
					FullTemplate: &Template{
						TemplateStr: componentTemplate,
					},
					engine: definition.NewWorkloadAbstractEngine("my-webservice"),
				},
			},
			WorkflowSteps: []wfTypesv1alpha1.WorkflowStep{
				{
					WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
						Name: "apply-microservice",
						Type: "apply-component",
						Inputs: wfTypesv1alpha1.StepInputs{
							{
								From:         "dynamicValue",
								ParameterKey: "image",
							},
						},
					},
				},
			},
		}

		p := &Parser{}
		err := p.ValidateCUESchematicAppfile(appfile)
		assert.NoError(t, err, "Should pass when workflow supplies missing param and NO traits present")
	})

	t.Run("workflow supplies param - WITH traits - should PASS", func(t *testing.T) {
		appfile := &Appfile{
			Name:      "test-app",
			Namespace: "test-ns",
			ParsedComponents: []*Component{
				{
					Name:               "my-webservice",
					Type:               "webservice",
					CapabilityCategory: types.CUECategory,
					Params: map[string]any{
						"port": 80,
					},
					FullTemplate: &Template{
						TemplateStr: componentTemplate,
					},
					engine: definition.NewWorkloadAbstractEngine("my-webservice"),
					Traits: []*Trait{
						{
							Name:               "labels",
							CapabilityCategory: types.CUECategory,
							Template:           traitTemplate,
							Params: map[string]any{
								"key":   "release",
								"value": "stable",
							},
							engine: definition.NewTraitAbstractEngine("labels"),
						},
					},
				},
			},
			WorkflowSteps: []wfTypesv1alpha1.WorkflowStep{
				{
					WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
						Name: "apply-microservice",
						Type: "apply-component",
						Inputs: wfTypesv1alpha1.StepInputs{
							{
								From:         "dynamicValue",
								ParameterKey: "image",
							},
						},
					},
				},
			},
		}

		p := &Parser{}
		err := p.ValidateCUESchematicAppfile(appfile)
		assert.NoError(t, err, "Should pass when workflow supplies missing param even WITH traits")
	})

	t.Run("workflow supplies param with ENUM - should use first enum value", func(t *testing.T) {
		enumComponentTemplate := `
		parameter: {
			image: "nginx:latest" | "apache:latest" | "httpd:latest"
			port: int | *80
		}
		output: {
			apiVersion: "apps/v1"
			kind: "Deployment"
			spec: {
				template: {
					spec: {
						containers: [{
							name: "main"
							image: parameter.image
							ports: [{
								containerPort: parameter.port
							}]
						}]
					}
				}
			}
		}
	`

		appfile := &Appfile{
			Name:      "test-app",
			Namespace: "test-ns",
			ParsedComponents: []*Component{
				{
					Name:               "my-webservice",
					Type:               "webservice",
					CapabilityCategory: types.CUECategory,
					Params: map[string]any{
						"port": 80,
					},
					FullTemplate: &Template{
						TemplateStr: enumComponentTemplate,
					},
					engine: definition.NewWorkloadAbstractEngine("my-webservice"),
					Traits: []*Trait{
						{
							Name:               "labels",
							CapabilityCategory: types.CUECategory,
							Template:           traitTemplate,
							Params: map[string]any{
								"key":   "release",
								"value": "stable",
							},
							engine: definition.NewTraitAbstractEngine("labels"),
						},
					},
				},
			},
			WorkflowSteps: []wfTypesv1alpha1.WorkflowStep{
				{
					WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
						Name: "apply-microservice",
						Type: "apply-component",
						Inputs: wfTypesv1alpha1.StepInputs{
							{
								From:         "dynamicValue",
								ParameterKey: "image",
							},
						},
					},
				},
			},
		}

		p := &Parser{}
		err := p.ValidateCUESchematicAppfile(appfile)
		assert.NoError(t, err, "Should use first enum value as default")
	})

	t.Run("param missing everywhere - should FAIL", func(t *testing.T) {
		appfile := &Appfile{
			Name:      "test-app",
			Namespace: "test-ns",
			ParsedComponents: []*Component{
				{
					Name:               "my-webservice",
					Type:               "webservice",
					CapabilityCategory: types.CUECategory,
					Params: map[string]any{
						"port": 80,
					},
					FullTemplate: &Template{
						TemplateStr: componentTemplate,
					},
					engine: definition.NewWorkloadAbstractEngine("my-webservice"),
					Traits: []*Trait{
						{
							Name:               "labels",
							CapabilityCategory: types.CUECategory,
							Template:           traitTemplate,
							Params: map[string]any{
								"key":   "release",
								"value": "stable",
							},
							engine: definition.NewTraitAbstractEngine("labels"),
						},
					},
				},
			},
		}

		p := &Parser{}
		err := p.ValidateCUESchematicAppfile(appfile)
		assert.Error(t, err, "Should fail when param is missing everywhere")
		assert.Contains(t, err.Error(), "missing parameters: image")
	})

	t.Run("override policy supplies param - WITH traits - should PASS", func(t *testing.T) {
		policyJSON := `{
			"components": [{
				"properties": {
					"image": "nginx:1.20"
				}
			}]
		}`

		appfile := &Appfile{
			Name:      "test-app",
			Namespace: "test-ns",
			ParsedComponents: []*Component{
				{
					Name:               "my-webservice",
					Type:               "webservice",
					CapabilityCategory: types.CUECategory,
					Params: map[string]any{
						"port": 80,
					},
					FullTemplate: &Template{
						TemplateStr: componentTemplate,
					},
					engine: definition.NewWorkloadAbstractEngine("my-webservice"),
					Traits: []*Trait{
						{
							Name:               "labels",
							CapabilityCategory: types.CUECategory,
							Template:           traitTemplate,
							Params: map[string]any{
								"key":   "release",
								"value": "stable",
							},
							engine: definition.NewTraitAbstractEngine("labels"),
						},
					},
				},
			},
			Policies: []v1beta1.AppPolicy{
				{
					Name: "override-policy",
					Type: "override",
					Properties: &runtime.RawExtension{
						Raw: []byte(policyJSON),
					},
				},
			},
		}

		p := &Parser{}
		err := p.ValidateCUESchematicAppfile(appfile)
		assert.NoError(t, err, "Should pass when override policy supplies missing param")
	})

	t.Run("workflow supplies different param types - should use correct defaults", func(t *testing.T) {
		multiTypeTemplate := `
		parameter: {
			count: int
			enabled: bool
			tags: [...string]
			port: int | *80
		}
		output: {
			apiVersion: "v1"
			kind: "ConfigMap"
			data: {
				count: "\(parameter.count)"
				enabled: "\(parameter.enabled)"
				port: "\(parameter.port)"
			}
			metadata: {
				labels: {
					for i, tag in parameter.tags {
						"tag-\(i)": tag
					}
				}
			}
		}
	`

		appfile := &Appfile{
			Name:      "test-app",
			Namespace: "test-ns",
			ParsedComponents: []*Component{
				{
					Name:               "my-config",
					Type:               "raw",
					CapabilityCategory: types.CUECategory,
					Params: map[string]any{
						"port": 80,
					},
					FullTemplate: &Template{
						TemplateStr: multiTypeTemplate,
					},
					engine: definition.NewWorkloadAbstractEngine("my-config"),
					Traits: []*Trait{
						{
							Name:               "labels",
							CapabilityCategory: types.CUECategory,
							Template:           traitTemplate,
							Params: map[string]any{
								"key":   "env",
								"value": "test",
							},
							engine: definition.NewTraitAbstractEngine("labels"),
						},
					},
				},
			},
			WorkflowSteps: []wfTypesv1alpha1.WorkflowStep{
				{
					WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
						Name: "apply-config",
						Type: "apply-component",
						Inputs: wfTypesv1alpha1.StepInputs{
							{From: "dynamicCount", ParameterKey: "count"},
							{From: "dynamicEnabled", ParameterKey: "enabled"},
							{From: "dynamicTags", ParameterKey: "tags"},
						},
					},
				},
			},
		}

		p := &Parser{}
		err := p.ValidateCUESchematicAppfile(appfile)
		assert.NoError(t, err, "Should handle int, bool, list types with correct defaults")
	})

	t.Run("workflow supplies param with numeric bounds - should skip validation", func(t *testing.T) {
		// Component with complex validation that can't be easily defaulted
		complexTemplate := `
		parameter: {
			port: int & >1024 & <65535
			image: string
		}
		output: {
			apiVersion: "v1"
			kind: "Service"
			spec: {
				ports: [{
					port: parameter.port
				}]
			}
		}
	`

		appfile := &Appfile{
			Name:      "test-app",
			Namespace: "test-ns",
			ParsedComponents: []*Component{
				{
					Name:               "my-service",
					Type:               "service",
					CapabilityCategory: types.CUECategory,
					Params: map[string]any{
						"image": "nginx:latest",
					},
					FullTemplate: &Template{
						TemplateStr: complexTemplate,
					},
					engine: definition.NewWorkloadAbstractEngine("my-service"),
					Traits: []*Trait{
						{
							Name:               "labels",
							CapabilityCategory: types.CUECategory,
							Template:           traitTemplate,
							Params: map[string]any{
								"key":   "version",
								"value": "v1",
							},
							engine: definition.NewTraitAbstractEngine("labels"),
						},
					},
				},
			},
			WorkflowSteps: []wfTypesv1alpha1.WorkflowStep{
				{
					WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
						Name: "apply-service",
						Type: "apply-component",
						Inputs: wfTypesv1alpha1.StepInputs{
							{From: "dynamicPort", ParameterKey: "port"},
						},
					},
				},
			},
		}

		p := &Parser{}
		err := p.ValidateCUESchematicAppfile(appfile)
		// Should pass by skipping validation due to complex constraints
		assert.NoError(t, err, "Should skip validation when complex constraints cannot be satisfied")
	})

	t.Run("workflow param already provided in component - should not augment", func(t *testing.T) {
		appfile := &Appfile{
			Name:      "test-app",
			Namespace: "test-ns",
			ParsedComponents: []*Component{
				{
					Name:               "my-webservice",
					Type:               "webservice",
					CapabilityCategory: types.CUECategory,
					Params: map[string]any{
						"image": "custom-image:v1.0",
						"port":  8080,
					},
					FullTemplate: &Template{
						TemplateStr: componentTemplate,
					},
					engine: definition.NewWorkloadAbstractEngine("my-webservice"),
					Traits: []*Trait{
						{
							Name:               "labels",
							CapabilityCategory: types.CUECategory,
							Template:           traitTemplate,
							Params: map[string]any{
								"key":   "app",
								"value": "myapp",
							},
							engine: definition.NewTraitAbstractEngine("labels"),
						},
					},
				},
			},
			WorkflowSteps: []wfTypesv1alpha1.WorkflowStep{
				{
					WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
						Name: "apply-webservice",
						Type: "apply-component",
						Inputs: wfTypesv1alpha1.StepInputs{
							{From: "dynamicImage", ParameterKey: "image"},
						},
					},
				},
			},
		}

		p := &Parser{}
		err := p.ValidateCUESchematicAppfile(appfile)
		assert.NoError(t, err, "Should use existing param value, not augment from workflow")
	})
}

func TestGetTypeDefault(t *testing.T) {
	tests := []struct {
		name     string
		kind     cue.Kind
		expected interface{}
	}{
		{"string", cue.StringKind, "__workflow_supplied__"},
		{"float", cue.FloatKind, 0.0},
		{"int", cue.IntKind, 0},
		{"number", cue.NumberKind, 0},
		{"bool", cue.BoolKind, false},
		{"list", cue.ListKind, []interface{}{}},
		{"struct", cue.StructKind, map[string]interface{}{}},
		{"bottom (unknown)", cue.BottomKind, "__workflow_supplied__"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getTypeDefault(tc.kind)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetDefaultForMissingParameter(t *testing.T) {
	var r cue.Runtime

	t.Run("simple string type should return string default", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { name: string }`)
		assert.NoError(t, err)
		val := inst.Value().LookupPath(cue.ParsePath("parameter.name"))
		canDefault, defaultVal := getDefaultForMissingParameter(val)
		assert.True(t, canDefault)
		assert.Equal(t, "__workflow_supplied__", defaultVal)
	})

	t.Run("simple int type should return 0", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { count: int }`)
		assert.NoError(t, err)
		val := inst.Value().LookupPath(cue.ParsePath("parameter.count"))
		canDefault, defaultVal := getDefaultForMissingParameter(val)
		assert.True(t, canDefault)
		assert.Equal(t, 0, defaultVal)
	})

	t.Run("simple bool type should return false", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { enabled: bool }`)
		assert.NoError(t, err)
		val := inst.Value().LookupPath(cue.ParsePath("parameter.enabled"))
		canDefault, defaultVal := getDefaultForMissingParameter(val)
		assert.True(t, canDefault)
		assert.Equal(t, false, defaultVal)
	})

	t.Run("field with default should use default", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { port: int | *80 }`)
		assert.NoError(t, err)
		val := inst.Value().LookupPath(cue.ParsePath("parameter.port"))
		canDefault, _ := getDefaultForMissingParameter(val)
		assert.True(t, canDefault)
	})

	t.Run("concrete value should return true with nil default", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { port: 80 }`)
		assert.NoError(t, err)
		val := inst.Value().LookupPath(cue.ParsePath("parameter.port"))
		canDefault, defaultVal := getDefaultForMissingParameter(val)
		assert.True(t, canDefault)
		assert.Nil(t, defaultVal)
	})

	t.Run("enum disjunction should return first value", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { image: "nginx:latest" | "apache:latest" }`)
		assert.NoError(t, err)
		val := inst.Value().LookupPath(cue.ParsePath("parameter.image"))
		canDefault, defaultVal := getDefaultForMissingParameter(val)
		assert.True(t, canDefault)
		assert.Equal(t, "nginx:latest", defaultVal)
	})

	t.Run("list type should be defaultable", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { tags: [...string] }`)
		assert.NoError(t, err)
		val := inst.Value().LookupPath(cue.ParsePath("parameter.tags"))
		canDefault, _ := getDefaultForMissingParameter(val)
		assert.True(t, canDefault)
	})
}

func TestGetMapKeys(t *testing.T) {
	t.Run("should return all keys", func(t *testing.T) {
		m := map[string]interface{}{
			"a": 1,
			"b": 2,
			"c": 3,
		}
		keys := getMapKeys(m)
		assert.Len(t, keys, 3)
		assert.ElementsMatch(t, []string{"a", "b", "c"}, keys)
	})

	t.Run("should return empty slice for empty map", func(t *testing.T) {
		keys := getMapKeys(map[string]interface{}{})
		assert.Empty(t, keys)
	})
}

func TestGetWorkflowAndPolicySuppliedParams(t *testing.T) {
	t.Run("should collect from workflow step inputs", func(t *testing.T) {
		app := &Appfile{
			WorkflowSteps: []wfTypesv1alpha1.WorkflowStep{
				{
					WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
						Name: "step1",
						Type: "apply-component",
						Inputs: wfTypesv1alpha1.StepInputs{
							{From: "dynamic", ParameterKey: "image"},
							{From: "dynamic", ParameterKey: "port"},
						},
					},
				},
			},
		}
		result := getWorkflowAndPolicySuppliedParams(app)
		assert.True(t, result["image"])
		assert.True(t, result["port"])
		assert.False(t, result["other"])
	})

	t.Run("should collect from override policies", func(t *testing.T) {
		policyJSON := `{"components": [{"properties": {"image": "nginx:1.20", "replicas": 3}}]}`
		app := &Appfile{
			Policies: []v1beta1.AppPolicy{
				{
					Name: "override",
					Type: "override",
					Properties: &runtime.RawExtension{
						Raw: []byte(policyJSON),
					},
				},
			},
		}
		result := getWorkflowAndPolicySuppliedParams(app)
		assert.True(t, result["image"])
		assert.True(t, result["replicas"])
	})

	t.Run("should skip non-override policies", func(t *testing.T) {
		app := &Appfile{
			Policies: []v1beta1.AppPolicy{
				{
					Name: "topology",
					Type: "topology",
					Properties: &runtime.RawExtension{
						Raw: []byte(`{"clusters": ["local"]}`),
					},
				},
			},
		}
		result := getWorkflowAndPolicySuppliedParams(app)
		assert.Empty(t, result)
	})

	t.Run("should return empty for empty appfile", func(t *testing.T) {
		result := getWorkflowAndPolicySuppliedParams(&Appfile{})
		assert.Empty(t, result)
	})

	t.Run("should skip malformed policy JSON gracefully", func(t *testing.T) {
		app := &Appfile{
			Policies: []v1beta1.AppPolicy{
				{
					Name: "bad-override",
					Type: "override",
					Properties: &runtime.RawExtension{
						Raw: []byte(`{invalid json`),
					},
				},
			},
		}
		result := getWorkflowAndPolicySuppliedParams(app)
		assert.Empty(t, result)
	})
}

func TestGetDeclaredFieldNames(t *testing.T) {
	var r cue.Runtime

	t.Run("should return all declared field names including optional", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: { name: string, label?: string, count: int }`)
		assert.NoError(t, err)
		schema := inst.Value().LookupPath(cue.ParsePath("parameter"))
		declared := getDeclaredFieldNames(schema)
		assert.True(t, declared["name"])
		assert.True(t, declared["label"])
		assert.True(t, declared["count"])
		assert.False(t, declared["nonexistent"])
	})

	t.Run("should return empty map for non-struct", func(t *testing.T) {
		inst, err := r.Compile("", `parameter: string`)
		assert.NoError(t, err)
		schema := inst.Value().LookupPath(cue.ParsePath("parameter"))
		declared := getDeclaredFieldNames(schema)
		assert.Empty(t, declared)
	})
}
