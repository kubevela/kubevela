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

package utils

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kubevela/pkg/cue/cuex"
	"github.com/kubevela/pkg/util/singleton"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"cuelang.org/go/cue/errors"
	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/core"
)

func TestValidateDefinitionRevision(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	v1beta1.AddToScheme(scheme)

	baseCompDef := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-def",
			Namespace: "default",
		},
		Spec: v1beta1.ComponentDefinitionSpec{
			Workload: common.WorkloadTypeDescriptor{
				Definition: common.WorkloadGVK{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				},
			},
			Schematic: &common.Schematic{
				CUE: &common.CUE{
					Template: `
output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: name: context.name
}`,
				},
			},
		},
	}

	expectedDefRev, _, err := core.GatherRevisionInfo(baseCompDef)
	assert.NoError(t, err, "Setup: failed to gather revision info")
	expectedDefRev.Name = "test-def-v1"
	expectedDefRev.Namespace = "default"

	mismatchedHashDefRev := expectedDefRev.DeepCopy()
	mismatchedHashDefRev.Spec.RevisionHash = "different-hash"

	mismatchedSpecDefRev := expectedDefRev.DeepCopy()
	mismatchedSpecDefRev.Spec.ComponentDefinition.Spec.Workload.Definition.Kind = "StatefulSet"

	// tweakedCompDef := baseCompDef.DeepCopy()
	// tweakedCompDef.Spec.Schematic.CUE.Template = `
	// output: {
	// 	apiVersion: "apps/v1"
	// 	kind: "Deployment"
	// 	metadata: name: context.name
	// 	// a tweak
	// }`
	testCases := map[string]struct {
		def                 runtime.Object
		defRevName          types.NamespacedName
		existingObjs        []runtime.Object
		expectErr           bool
		expectedErrContains string
	}{
		"Success with matching definition revision": {
			def:          baseCompDef,
			defRevName:   types.NamespacedName{Name: "test-def-v1", Namespace: "default"},
			existingObjs: []runtime.Object{expectedDefRev},
			expectErr:    false,
		},
		"Success when definition revision does not exist": {
			def:          baseCompDef,
			defRevName:   types.NamespacedName{Name: "test-def-v1", Namespace: "default"},
			existingObjs: []runtime.Object{},
			expectErr:    false,
		},
		"Failure with revision hash mismatch": {
			def:                 baseCompDef,
			defRevName:          types.NamespacedName{Name: "test-def-v1", Namespace: "default"},
			existingObjs:        []runtime.Object{mismatchedHashDefRev},
			expectErr:           true,
			expectedErrContains: "the definition's spec is different with existing definitionRevision's spec",
		},
		"Failure with spec mismatch (DeepEqual)": {
			def:                 baseCompDef,
			defRevName:          types.NamespacedName{Name: "test-def-v1", Namespace: "default"},
			existingObjs:        []runtime.Object{mismatchedSpecDefRev},
			expectErr:           true,
			expectedErrContains: "the definition's spec is different with existing definitionRevision's spec",
		},
		"Failure with invalid definition revision name": {
			def:                 baseCompDef,
			defRevName:          types.NamespacedName{Name: "invalid!name", Namespace: "default"},
			existingObjs:        []runtime.Object{},
			expectErr:           true,
			expectedErrContains: "invalid definitionRevision name",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cli := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tc.existingObjs...).
				Build()

			err := ValidateDefinitionRevision(context.Background(), cli, tc.def, tc.defRevName)

			if tc.expectErr {
				assert.Error(t, err)
				if tc.expectedErrContains != "" {
					assert.Contains(t, err.Error(), tc.expectedErrContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCueTemplate(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		cueTemplate string
		want        error
	}{
		"normalCueTemp": {
			cueTemplate: "name: 'name'",
			want:        nil,
		},
		"contextNouFoundCueTemp": {
			cueTemplate: `
				output: {
					metadata: {
						name: context.name
						label: context.label
						annotation: "default"
					}
				}`,
			want: nil,
		},
		"inValidCueTemp": {
			cueTemplate: `
				output: {
					metadata: {
						name: context.name
						label: context.label
						annotation: "default"
					},
					hello: world 
				}`,
			want: errors.New("output.hello: reference \"world\" not found"),
		},
		"emptyCueTemp": {
			cueTemplate: "",
			want:        nil,
		},
		"malformedCueTemp": {
			cueTemplate: "output: { metadata: { name: context.name, label: context.label, annotation: \"default\" }, hello: world ",
			want:        errors.New("expected '}', found 'EOF'"),
		},
	}

	for caseName, cs := range cases {
		t.Run(caseName, func(t *testing.T) {
			t.Parallel()
			err := ValidateCueTemplate(cs.cueTemplate)
			if cs.want != nil {
				assert.EqualError(t, cs.want, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCuexTemplate(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		cueTemplate string
		want        error
	}{
		"normalCueTemp": {
			cueTemplate: "name: 'name'",
			want:        nil,
		},
		"contextNouFoundCueTemp": {
			cueTemplate: `
				output: {
					metadata: {
						name: context.name
						label: context.label
						annotation: "default"
					}
				}`,
			want: nil,
		},
		"withCuexPackageImports": {
			cueTemplate: `
				import "test/ext"
				
				test: ext.#Add & {
					a: 1
					b: 2
				}
	
				output: {
					metadata: {
						name: context.name + "\(test.result)"
						label: context.label
						annotation: "default"
					}
				}
			`,
			want: nil,
		},
		"inValidCueTemp": {
			cueTemplate: `
				output: {
					metadata: {
						name: context.name
						label: context.label
						annotation: "default"
					},
					hello: world 
				}`,
			want: errors.New("output.hello: reference \"world\" not found"),
		},
	}

	packageObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cue.oam.dev/v1alpha1",
			"kind":       "Package",
			"metadata": map[string]interface{}{
				"name":      "test-package",
				"namespace": "vela-system",
			},
			"spec": map[string]interface{}{
				"path": "test/ext",
				"templates": map[string]interface{}{
					"test/ext": strings.TrimSpace(`
                        package ext
                        #Add: {
						  a: number
						  b: number
                          result: a + b
						}
                    `),
				},
			},
		},
	}

	dcl := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), packageObj)
	singleton.DynamicClient.Set(dcl)
	cuex.DefaultCompiler.Reload()

	defer singleton.ReloadClients()
	defer cuex.DefaultCompiler.Reload()

	for caseName, cs := range cases {
		t.Run(caseName, func(t *testing.T) {
			t.Parallel()
			err := ValidateCuexTemplate(context.Background(), cs.cueTemplate)
			if cs.want != nil {
				assert.Equal(t, cs.want.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSemanticVersion(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		version string
		want    error
	}{
		"validVersion": {
			version: "1.2.3",
			want:    nil,
		},
		"versionWithAlphabets": {
			version: "1.2.3-alpha",
			want:    errors.New("Not a valid version"),
		},
		"invalidVersion": {
			version: "1.2",
			want:    errors.New("Not a valid version"),
		},
	}
	for caseName, cs := range cases {
		t.Run(caseName, func(t *testing.T) {
			t.Parallel()
			err := ValidateSemanticVersion(cs.version)
			if cs.want != nil {
				assert.Error(t, err)
				assert.EqualError(t, cs.want, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateMultipleDefVersionsNotPresent(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		version      string
		revisionName string
		want         error
	}{
		"versionPresent": {
			version:      "1.2.3",
			revisionName: "",
			want:         nil,
		},
		"revisionNamePresent": {
			version:      "",
			revisionName: "2.3",
			want:         nil,
		},
		"versionAndRevisionNamePresent": {
			version:      "1.2.3",
			revisionName: "2.3",
			want:         fmt.Errorf("ComponentDefinition has both spec.version and revision name annotation. Only one can be present"),
		},
	}
	for caseName, cs := range cases {
		t.Run(caseName, func(t *testing.T) {
			t.Parallel()
			err := ValidateMultipleDefVersionsNotPresent(cs.version, cs.revisionName, "ComponentDefinition")
			if cs.want != nil {
				assert.Error(t, err)
				assert.EqualError(t, cs.want, err.Error())
			} else {
				assert.NoError(t, err)
			}

		})
	}
}
