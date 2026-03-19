/*
Copyright 2026 The KubeVela Authors.

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

package application

import (
	"context"
	"fmt"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
)

// dryRunCaptureKey is the context key used to pass a *dryRunCapture into
// ApplyApplicationScopeTransforms so rendered results can be retrieved by the
// caller without a cache round-trip (the cache key changes after spec mutation).
type dryRunCaptureKeyType struct{}

var dryRunCaptureKey = dryRunCaptureKeyType{}

// dryRunCapture holds rendered results written by ApplyApplicationScopeTransforms
// when a dry-run context is active.
type dryRunCapture struct {
	results []RenderedPolicyResult
}

// PolicyDryRunResult contains the results of a policy dry-run simulation
type PolicyDryRunResult struct {
	// Application is the final in-memory state after all policies applied
	Application *v1beta1.Application
	// PolicyResults contains per-policy results in execution order
	PolicyResults []common.AppliedApplicationPolicy
	// PolicyDetails contains the full per-policy observability data (same structure as the
	// ConfigMap written by the controller), keyed by policy name. Includes SpecBefore/SpecAfter.
	PolicyDetails map[string]map[string]interface{}
	// FinalContext is the merged context injected by all policies (union of output.ctx fields).
	// This is what downstream CUE templates would see as context.custom.* during rendering.
	FinalContext map[string]interface{}
	// Errors contains any errors encountered during simulation
	Errors []string
}

// SimulatePolicyApplication performs a dry-run simulation of policy application.
// It runs the exact same code path as the controller (ApplyApplicationScopeTransforms)
// on a deep copy of the Application, so results are guaranteed to match what the
// controller would do.
func SimulatePolicyApplication(ctx context.Context, cli client.Client, app *v1beta1.Application) (*PolicyDryRunResult, error) {
	// Dry-run bypasses feature gates so users can simulate policies regardless of
	// whether EnableApplicationScopedPolicies / EnableGlobalPolicies are enabled.
	origASP := utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableApplicationScopedPolicies)
	origGP := utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableGlobalPolicies)
	_ = utilfeature.DefaultMutableFeatureGate.Set("EnableApplicationScopedPolicies=true,EnableGlobalPolicies=true")
	defer utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("EnableApplicationScopedPolicies=%v,EnableGlobalPolicies=%v", origASP, origGP)) //nolint:errcheck

	// Seed the policy scope index from the cluster so explicit policy lookup works
	// without a running controller watch.
	pdList := &v1beta1.PolicyDefinitionList{}
	if err := cli.List(ctx, pdList); err == nil {
		for i := range pdList.Items {
			policyScopeIndex.AddOrUpdate(&pdList.Items[i])
		}
	}

	appCopy := app.DeepCopy()

	// Attach a capture so ApplyApplicationScopeTransforms can stash rendered results
	// before the spec is mutated (the cache key changes after mutation).
	capture := &dryRunCapture{}
	baseCtx := context.WithValue(ctx, dryRunCaptureKey, capture)
	monCtx := monitorContext.NewTraceContext(baseCtx, "policy-dry-run")

	handler := &AppHandler{
		Client: cli,
		app:    appCopy,
	}

	result := &PolicyDryRunResult{
		Application:   appCopy,
		PolicyResults: []common.AppliedApplicationPolicy{},
		Errors:        []string{},
	}

	_, err := handler.ApplyApplicationScopeTransforms(monCtx, appCopy)
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
		return result, nil // Return partial results rather than failing entirely
	}

	result.PolicyResults = appCopy.Status.AppliedApplicationPolicies

	// Populate PolicyDetails from the captured rendered results.
	if len(capture.results) > 0 {
		result.PolicyDetails = make(map[string]map[string]interface{}, len(capture.results))
		mergedCtx := make(map[string]interface{})
		for _, r := range capture.results {
			result.PolicyDetails[r.PolicyName] = buildPolicyObservabilityData(r)
			for k, v := range r.AdditionalContext {
				mergedCtx[k] = v
			}
		}
		if len(mergedCtx) > 0 {
			result.FinalContext = mergedCtx
		}
	}

	return result, nil
}
