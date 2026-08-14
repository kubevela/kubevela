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
	"os"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"cuelang.org/go/cue/errors"
	"github.com/kubevela/pkg/cue/cuex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/core"
)

// TestMain keeps the validation tests hermetic. Building the compiler otherwise
// loads external Package CRDs from the cluster, and that path reaches
// singleton.KubeConfig -> config.GetConfigOrDie(), which terminates the process
// with os.Exit(1) when no kubeconfig is reachable rather than returning an
// error. Admission itself still loads external packages in a real deployment;
// only the tests opt out so they can run without a cluster.
func TestMain(m *testing.M) {
	cuex.EnableExternalPackageForDefaultCompiler = false
	cuex.EnableExternalPackageWatchForDefaultCompiler = false
	os.Exit(m.Run())
}

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
		// The upstream `withCuexPackageImports` case relied on
		// cuex.DefaultCompiler.Reload picking up a fake-client-served
		// Package CRD. External package loading is disabled outright in
		// TestMain so these tests can run without a cluster, so that case
		// cannot be expressed here. Internal-package import resolution is
		// covered instead by TestValidateWorkflowStepCuexTemplate, which
		// compiles templates importing vela/op and vela/kube.
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

// TestValidateWorkflowStepCuexTemplate covers ValidateWorkflowStepCuexTemplate,
// which compiles against the workflow provider compiler rather than the
// workload one that ValidateCuexTemplate uses. The workload compiler rejects
// any template importing a workflow-only builtin package, which is most of
// the bundled step definitions.
func TestValidateWorkflowStepCuexTemplate(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		cueTemplate string
		wantErr     string
	}{
		"velaOpImportIsAccepted": {
			cueTemplate: `
import "vela/op"

wait: op.#ConditionalWait & {
	continue: true
}`,
		},
		"velaKubeImportIsAccepted": {
			cueTemplate: `
import "vela/kube"

output: kube.#Read & {
	$params: value: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name:      "cm"
			namespace: "default"
		}
	}
}`,
		},
		// The problem reported in the issue: an unused import is invalid CUE,
		// and a WorkflowStepDefinition carrying one used to be admitted.
		"unusedImportIsRejected": {
			cueTemplate: `
import "vela/kube"

parameter: {
	name: string
}`,
			wantErr: `imported and not used: "vela/kube"`,
		},
		"unknownPackageIsRejected": {
			cueTemplate: `
import "vela/doesnotexist"

output: doesnotexist.#Foo`,
			wantErr: `builtin package "vela/doesnotexist" undefined`,
		},
		// Provider function arguments are still checked even though the
		// functions are never executed, because imports and package schemas
		// resolve during BuildInstance.
		"badProviderArgIsRejected": {
			cueTemplate: `
import "vela/kube"

output: kube.#Read & {
	$params: value: 12345
}`,
			wantErr: "output.$params.value: conflicting values 12345 and {...} (mismatched types int and struct)",
		},
		// Guarding on an unresolved provider result must not fail validation.
		// depends-on-app does exactly this, and with resolution disabled the
		// comprehension simply stays unevaluated.
		"returnsGuardStaysIncomplete": {
			cueTemplate: `
import "vela/kube"

dependsOn: kube.#Read & {
	$params: value: {
		apiVersion: "core.oam.dev/v1beta1"
		kind:       "Application"
		metadata: {
			name:      "app"
			namespace: "default"
		}
	}
}
load: {
	if dependsOn.$returns.err != _|_ {
		found: true
	}
}`,
		},
	}

	for caseName, cs := range cases {
		t.Run(caseName, func(t *testing.T) {
			t.Parallel()
			err := ValidateWorkflowStepCuexTemplate(context.Background(), cs.cueTemplate)
			if cs.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			// require, not assert: if the validator wrongly returns nil here,
			// err.Error() below would panic on a nil receiver and the panic
			// message would replace the real "expected an error" failure,
			// making the test output misleading about what actually failed.
			require.Error(t, err)
			assert.Contains(t, err.Error(), cs.wantErr)
		})
	}
}

// TestValidateCuexTemplate_DoesNotExecuteProviders asserts the safety property
// the DisableResolveProviderFunctions option buys. A template supplying fully
// concrete arguments to a provider must be type-checked without the provider
// ever running, since admission must not perform cluster reads or chart
// fetches as a side effect of submitting a definition.
func TestValidateWorkflowStepCuexTemplate_DoesNotExecuteProviders(t *testing.T) {
	t.Parallel()
	// kube.#Read against a cluster that is not reachable from this test
	// process. If resolution ran, this would attempt a real read and fail
	// (or terminate the process via GetConfigOrDie).
	concrete := `
import "vela/kube"

output: kube.#Read & {
	$params: {
		cluster: "some-remote-cluster"
		value: {
			apiVersion: "v1"
			kind:       "Secret"
			metadata: {
				name:      "does-not-exist"
				namespace: "does-not-exist"
			}
		}
	}
}`
	assert.NoError(t, ValidateWorkflowStepCuexTemplate(context.Background(), concrete))
}

// TestCuexTemplateValidators_AreNotInterchangeable pins the reason there are two
// validators rather than one shared "superset" compiler.
//
// "vela/kube", "vela/http" and "vela/config" are registered by both compilers
// but resolve to different packages with different selectors and different
// $params schemas. Using the wrong one does not error. The selector is silently
// accepted and the $params schema check vanishes with it, turning a checked
// template into an unchecked one.
//
// No bundled ComponentDefinition or TraitDefinition imports those three paths,
// so a suite that only walks the bundled definitions reports everything green
// under either compiler. These cases use the clashing imports directly so the
// regression is actually observable.
func TestCuexTemplateValidators_AreNotInterchangeable(t *testing.T) {
	t.Parallel()

	// kube.#Get is workload-flavor; $params.resource belongs to it.
	workloadFlavor := `
import "vela/kube"

output: kube.#Get & {
	$params: {
		resource: {
			apiVersion: "v1"
			kind:       "ConfigMap"
		}
		totallyBogusField: "x"
	}
}`

	// kube.#Read is workflow-flavor; $params.value belongs to it.
	workflowFlavor := `
import "vela/kube"

output: kube.#Read & {
	$params: {
		value: {
			apiVersion: "v1"
			kind:       "ConfigMap"
		}
		totallyBogusField: "x"
	}
}`

	t.Run("component validator catches a bogus workload-flavor param", func(t *testing.T) {
		t.Parallel()
		err := ValidateCuexTemplate(context.Background(), workloadFlavor)
		assert.Error(t, err, "workload compiler must type-check its own $params schema")
		assert.Contains(t, err.Error(), "field not allowed")
	})

	t.Run("step validator catches a bogus workflow-flavor param", func(t *testing.T) {
		t.Parallel()
		err := ValidateWorkflowStepCuexTemplate(context.Background(), workflowFlavor)
		assert.Error(t, err, "workflow compiler must type-check its own $params schema")
		assert.Contains(t, err.Error(), "field not allowed")
	})

	// The two below document the silent-loss behaviour that makes routing matter.
	// They are not asserting desirable behaviour, they are pinning the hazard so
	// that collapsing these validators back into one fails loudly here.
	t.Run("step validator does not catch a workload-flavor param", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, ValidateWorkflowStepCuexTemplate(context.Background(), workloadFlavor),
			"if this now errors, the flavors converged and the split may be revisitable")
	})

	t.Run("component validator does not catch a workflow-flavor param", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, ValidateCuexTemplate(context.Background(), workflowFlavor),
			"if this now errors, the flavors converged and the split may be revisitable")
	})
}
