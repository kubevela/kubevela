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
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func TestValidateCueTemplate(t *testing.T) {
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
	}

	for caseName, cs := range cases {
		t.Run(caseName, func(t *testing.T) {
			err := ValidateCueTemplate(cs.cueTemplate)
			if diff := cmp.Diff(cs.want, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nValidateCueTemplate: -want , +got \n%s\n", cs.want, diff)
			}
		})
	}
}

func TestValidateCuexTemplate(t *testing.T) {
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
			err := ValidateCuexTemplate(context.Background(), cs.cueTemplate)
			if diff := cmp.Diff(cs.want, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nValidateCueTemplate: -want , +got \n%s\n", cs.want, diff)
			}
		})
	}
}

func TestValidateSemanticVersion(t *testing.T) {
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
			err := ValidateSemanticVersion(cs.version)
			if cs.want != nil {
				assert.Equal(t, err.Error(), cs.want.Error())
			} else {
				assert.Equal(t, err, cs.want)
			}

		})
	}
}

func TestValidateMultipleDefVersionsNotPresent(t *testing.T) {
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
			err := ValidateMultipleDefVersionsNotPresent(cs.version, cs.revisionName, "ComponentDefinition")
			if cs.want != nil {
				assert.Equal(t, err.Error(), cs.want.Error())
			} else {
				assert.Equal(t, err, cs.want)
			}

		})
	}
}
