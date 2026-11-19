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
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
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
	// Flags are the controller flags the simulation assumed.
	Flags PolicyDryRunFlags
}

// PolicyDryRunFlags are the controller flags that change which policies apply. A simulation reads
// them from this process (AllowNamespaceGlobalPolicies and AllowSkipGlobalPolicies in
// pkg/controller/common), so they match the controller's only when the caller sets them to its
// values.
type PolicyDryRunFlags struct {
	AllowNamespaceGlobalPolicies bool `json:"allowNamespaceGlobalPolicies"`
	AllowSkipGlobalPolicies      bool `json:"allowSkipGlobalPolicies"`
}

// SimulatePolicyApplication performs a dry-run simulation of policy application.
// It runs the controller's code path (ApplyApplicationScopeTransforms) on a deep copy of the
// Application, through a client that drops every write. The result matches the controller's when
// this process holds the controller's flags (see PolicyDryRunFlags) and the policies read the
// same cluster data.
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
	// A simulation always renders: the cache is keyed on the app alone, not on the flags.
	applicationPolicyCache.InvalidateApplication(appCopy.Namespace, appCopy.Name)

	// ApplyApplicationScopeTransforms stashes the results the controller would apply, held data
	// included, in the capture before it mutates the spec.
	capture := &dryRunCapture{}
	baseCtx := context.WithValue(ctx, dryRunCaptureKey, capture)
	monCtx := monitorContext.NewTraceContext(baseCtx, "policy-dry-run")

	handler := &AppHandler{
		Client: readOnlyClient{Client: cli},
		app:    appCopy,
	}

	result := &PolicyDryRunResult{
		Application:   appCopy,
		PolicyResults: []common.AppliedApplicationPolicy{},
		Errors:        []string{},
		Flags: PolicyDryRunFlags{
			AllowNamespaceGlobalPolicies: common2.AllowNamespaceGlobalPolicies,
			AllowSkipGlobalPolicies:      common2.AllowSkipGlobalPolicies,
		},
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
			result.PolicyDetails[r.PolicyName] = buildPolicyObservabilityData(appCopy, r)
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

// readOnlyClient reads through to the cluster and drops every write, so a simulation changes
// nothing there.
type readOnlyClient struct {
	client.Client
}

func (readOnlyClient) Create(context.Context, client.Object, ...client.CreateOption) error {
	return nil
}

func (readOnlyClient) Update(context.Context, client.Object, ...client.UpdateOption) error {
	return nil
}

func (readOnlyClient) Patch(context.Context, client.Object, client.Patch, ...client.PatchOption) error {
	return nil
}

func (readOnlyClient) Apply(context.Context, runtime.ApplyConfiguration, ...client.ApplyOption) error {
	return nil
}

func (readOnlyClient) Delete(context.Context, client.Object, ...client.DeleteOption) error {
	return nil
}

func (readOnlyClient) DeleteAllOf(context.Context, client.Object, ...client.DeleteAllOfOption) error {
	return nil
}

func (c readOnlyClient) Status() client.SubResourceWriter { return c.SubResource("status") }

func (c readOnlyClient) SubResource(name string) client.SubResourceClient {
	return readOnlySubResource{SubResourceClient: c.Client.SubResource(name)}
}

// readOnlySubResource reads a subresource through to the cluster and drops every write.
type readOnlySubResource struct {
	client.SubResourceClient
}

func (readOnlySubResource) Create(context.Context, client.Object, client.Object, ...client.SubResourceCreateOption) error {
	return nil
}

func (readOnlySubResource) Update(context.Context, client.Object, ...client.SubResourceUpdateOption) error {
	return nil
}

func (readOnlySubResource) Patch(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
	return nil
}

func (readOnlySubResource) Apply(context.Context, runtime.ApplyConfiguration, ...client.SubResourceApplyOption) error {
	return nil
}
