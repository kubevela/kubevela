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

package step

import (
	"bytes"
	"encoding/json"

	"context"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	types2 "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// LoadExternalPoliciesForWorkflow detects policies used in workflow steps which are not declared in internal policies
// try to load them from external policy objects in the application's namespace
func LoadExternalPoliciesForWorkflow(ctx context.Context, cli client.Client, appNs string, steps []wfTypesv1alpha1.WorkflowStep, internalPolicies []v1beta1.AppPolicy) ([]v1beta1.AppPolicy, error) {
	policies := internalPolicies
	policyMap := map[string]struct{}{}
	for _, policy := range policies {
		policyMap[policy.Name] = struct{}{}
	}
	// Load extra used policies declared in the workflow step
	for _, _step := range steps {
		if _step.Type == DeployWorkflowStep && _step.Properties != nil {
			props := DeployWorkflowStepSpec{}
			if err := utils.StrictUnmarshal(_step.Properties.Raw, &props); err != nil {
				// This decode happens while the appfile is built, before property
				// expressions are substituted, and it exists only to find the
				// policy names. A typed field carrying an expression is still a
				// string here - `parallelism: "$(source.x.count)"` - so strict
				// decoding rejected the step before anything could resolve it.
				//
				// Falling back to a policies-only decode keeps the strict check
				// for everything that parses today: the fallback is reached only
				// when the properties carry an expression, and the step's real
				// contract is still enforced against its CUE parameter block at
				// admission and again at render.
				if !propertiesCarryExpression(_step.Properties.Raw) {
					return nil, errors.Wrapf(err, "invalid WorkflowStep %s", _step.Name)
				}
				// Still strict about *which* fields exist - only lenient about
				// their types, since an expression has not been substituted yet.
				// Dropping the field check outright let `policys:` through
				// whenever any expression was present.
				untyped := deployStepFieldNames{}
				dec := json.NewDecoder(bytes.NewReader(_step.Properties.Raw))
				dec.DisallowUnknownFields()
				if jerr := dec.Decode(&untyped); jerr != nil {
					return nil, errors.Wrapf(jerr, "invalid WorkflowStep %s", _step.Name)
				}
				if len(untyped.Policies) > 0 {
					if perr := json.Unmarshal(untyped.Policies, &props.Policies); perr != nil {
						return nil, errors.Wrapf(perr, "invalid WorkflowStep %s", _step.Name)
					}
				}
			}
			for _, policyName := range props.Policies {
				// A name that is still an expression names nothing yet. Strict
				// decoding accepts it - it is a string, whatever it holds - so
				// looking it up here reported "external policy
				// $(source.cfg.name) not found" and failed the appfile before
				// the step that resolves it ever ran.
				if propexpr.HasExpression(policyName) {
					continue
				}
				if _, found := policyMap[policyName]; !found {
					po := &v1alpha1.Policy{}
					if err := cli.Get(ctx, types2.NamespacedName{Namespace: appNs, Name: policyName}, po); err != nil {
						if kerrors.IsNotFound(err) {
							return nil, errors.Errorf("external policy %s not found", policyName)
						}
						return nil, errors.Wrapf(err, "failed to load external policy %s in namespace %s", policyName, appNs)
					}
					policies = append(policies, v1beta1.AppPolicy{Name: policyName, Type: po.Type, Properties: po.Properties})
					policyMap[policyName] = struct{}{}
				}
			}
		}
	}
	return policies, nil
}

// deployStepFieldNames mirrors DeployWorkflowStepSpec's field names with their
// types erased, so unknown fields are still rejected while a field holding an
// unsubstituted expression is not.
//
// It has to be kept in step with DeployWorkflowStepSpec; the test below fails if
// a field is added there and not here.
type deployStepFieldNames struct {
	Auto                     json.RawMessage `json:"auto,omitempty"`
	Policies                 json.RawMessage `json:"policies,omitempty"`
	Parallelism              json.RawMessage `json:"parallelism,omitempty"`
	IgnoreTerraformComponent json.RawMessage `json:"ignoreTerraformComponent,omitempty"`
}

// propertiesCarryExpression reports whether a properties blob contains a $(...)
// property expression.
//
// Only used to decide whether a decode failure is worth reporting: an expression
// occupies a typed field as a string until it is substituted, so a failure with
// one present says nothing about whether the author got the step right.
func propertiesCarryExpression(raw []byte) bool {
	if !bytes.Contains(raw, []byte("$(")) {
		return false
	}
	var decoded interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return false
	}
	return propexpr.HasExpression(decoded)
}
