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

package application

import (
	"context"
	"encoding/json"
	"fmt"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// PolicyDryRunMode defines how policies are discovered and applied in dry-run
type PolicyDryRunMode string

const (
	// DryRunModeIsolated tests only specified policies
	DryRunModeIsolated PolicyDryRunMode = "isolated"
	// DryRunModeAdditive tests specified policies with existing globals
	DryRunModeAdditive PolicyDryRunMode = "additive"
	// DryRunModeFull simulates complete policy chain (globals + app policies)
	DryRunModeFull PolicyDryRunMode = "full"
)

// PolicyDryRunOptions contains configuration for policy dry-run simulation
type PolicyDryRunOptions struct {
	// Mode determines which policies are applied
	Mode PolicyDryRunMode
	// SpecifiedPolicies are the policy names to test (for isolated/additive modes)
	SpecifiedPolicies []string
	// IncludeAppPolicies includes policies from Application spec (for full mode)
	IncludeAppPolicies bool
}

// PolicyDryRunResult contains the results of a policy dry-run simulation
type PolicyDryRunResult struct {
	// Application is the final state after all policies applied
	Application *v1beta1.Application
	// ExecutionPlan shows which policies were discovered and in what order
	ExecutionPlan []PolicyExecutionStep
	// PolicyResults contains detailed results for each policy
	PolicyResults []PolicyApplicationResult
	// Diffs contains the JSON patches for each policy that modified the spec
	Diffs map[string][]byte
	// Warnings contains any warnings detected during simulation
	Warnings []string
	// Errors contains any errors encountered
	Errors []string
}

// PolicyExecutionStep represents a policy in the execution plan
type PolicyExecutionStep struct {
	Sequence        int
	PolicyName      string
	PolicyNamespace string
	Priority        int32
	Source          string // "global", "app-spec", or "specified"
}

// PolicyApplicationResult contains the results of applying a single policy
type PolicyApplicationResult struct {
	Sequence          int
	PolicyName        string
	PolicyNamespace   string
	Priority          int32
	Enabled           bool
	Applied           bool
	SpecModified      bool
	AddedLabels       map[string]string
	AddedAnnotations  map[string]string
	AdditionalContext *runtime.RawExtension
	SkipReason        string
	Error             string
}

// SimulatePolicyApplication performs a dry-run simulation of policy application
// This function can be used by CLI tools to preview policy effects without persisting changes
func SimulatePolicyApplication(ctx context.Context, cli client.Client, app *v1beta1.Application, opts PolicyDryRunOptions) (*PolicyDryRunResult, error) {
	// Create a deep copy of the application to avoid modifying the original
	appCopy := app.DeepCopy()

	// Create a monitor context
	monCtx := monitorContext.NewTraceContext(ctx, "")

	// Create AppHandler for policy operations
	handler := &AppHandler{
		Client: cli,
		app:    appCopy,
	}

	result := &PolicyDryRunResult{
		Application:   appCopy,
		ExecutionPlan: []PolicyExecutionStep{},
		PolicyResults: []PolicyApplicationResult{},
		Diffs:         make(map[string][]byte),
		Warnings:      []string{},
		Errors:        []string{},
	}

	// Clear any existing policy status
	appCopy.Status.AppliedApplicationPolicies = nil

	// Step 1: Build execution plan based on mode
	var policiesToApply []v1beta1.PolicyDefinition
	var sequence int = 1

	switch opts.Mode {
	case DryRunModeIsolated:
		// Test only specified policies
		if len(opts.SpecifiedPolicies) == 0 {
			return nil, errors.New("isolated mode requires at least one policy to be specified")
		}
		for _, policyName := range opts.SpecifiedPolicies {
			// Try to load from vela-system first, then app namespace
			policy, err := loadPolicyDefinition(ctx, cli, policyName, oam.SystemDefinitionNamespace)
			if err != nil {
				// Try app namespace
				policy, err = loadPolicyDefinition(ctx, cli, policyName, appCopy.Namespace)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Policy %s not found in vela-system or %s", policyName, appCopy.Namespace))
					continue
				}
			}
			policiesToApply = append(policiesToApply, *policy)

			result.ExecutionPlan = append(result.ExecutionPlan, PolicyExecutionStep{
				Sequence:        sequence,
				PolicyName:      policy.Name,
				PolicyNamespace: policy.Namespace,
				Priority:        policy.Spec.Priority,
				Source:          "specified",
			})
			sequence++
		}

	case DryRunModeAdditive:
		// Include global policies + specified policies
		if !shouldSkipGlobalPolicies(appCopy) && utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableGlobalPolicies) {
			// Discover global policies
			globalPolicies, err := discoverAndDeduplicateGlobalPolicies(monCtx, cli, appCopy.Namespace)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to discover global policies: %v", err))
			} else {
				for _, policy := range globalPolicies {
					policiesToApply = append(policiesToApply, policy)
					result.ExecutionPlan = append(result.ExecutionPlan, PolicyExecutionStep{
						Sequence:        sequence,
						PolicyName:      policy.Name,
						PolicyNamespace: policy.Namespace,
						Priority:        policy.Spec.Priority,
						Source:          "global",
					})
					sequence++
				}
			}
		}

		// Add specified policies
		for _, policyName := range opts.SpecifiedPolicies {
			policy, err := loadPolicyDefinition(ctx, cli, policyName, oam.SystemDefinitionNamespace)
			if err != nil {
				policy, err = loadPolicyDefinition(ctx, cli, policyName, appCopy.Namespace)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Policy %s not found", policyName))
					continue
				}
			}
			policiesToApply = append(policiesToApply, *policy)
			result.ExecutionPlan = append(result.ExecutionPlan, PolicyExecutionStep{
				Sequence:        sequence,
				PolicyName:      policy.Name,
				PolicyNamespace: policy.Namespace,
				Priority:        policy.Spec.Priority,
				Source:          "specified",
			})
			sequence++
		}

	case DryRunModeFull:
		// Full simulation: global + app policies
		if !shouldSkipGlobalPolicies(appCopy) && utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableGlobalPolicies) {
			globalPolicies, err := discoverAndDeduplicateGlobalPolicies(monCtx, cli, appCopy.Namespace)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to discover global policies: %v", err))
			} else {
				for _, policy := range globalPolicies {
					policiesToApply = append(policiesToApply, policy)
					result.ExecutionPlan = append(result.ExecutionPlan, PolicyExecutionStep{
						Sequence:        sequence,
						PolicyName:      policy.Name,
						PolicyNamespace: policy.Namespace,
						Priority:        policy.Spec.Priority,
						Source:          "global",
					})
					sequence++
				}
			}
		}

		// Add app spec policies if requested
		if opts.IncludeAppPolicies {
			for _, policyRef := range appCopy.Spec.Policies {
				policy, err := loadPolicyDefinition(ctx, cli, policyRef.Type, appCopy.Namespace)
				if err != nil {
					policy, err = loadPolicyDefinition(ctx, cli, policyRef.Type, oam.SystemDefinitionNamespace)
					if err != nil {
						result.Errors = append(result.Errors, fmt.Sprintf("Policy %s not found", policyRef.Type))
						continue
					}
				}
				policiesToApply = append(policiesToApply, *policy)
				result.ExecutionPlan = append(result.ExecutionPlan, PolicyExecutionStep{
					Sequence:        sequence,
					PolicyName:      policy.Name,
					PolicyNamespace: policy.Namespace,
					Priority:        policy.Spec.Priority,
					Source:          "app-spec",
				})
				sequence++
			}
		}
	}

	// Step 2: Apply each policy and track results
	policySequence := 1
	for _, policy := range policiesToApply {
		// Take snapshot before applying policy (for future use in diff computation)
		_, err := deepCopyAppSpec(&appCopy.Spec)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to snapshot spec for policy %s: %v", policy.Name, err))
		}

		// Render the policy
		policyRef := v1beta1.AppPolicy{
			Name: policy.Name,
			Type: policy.Name,
		}

		renderedResult, err := handler.renderPolicy(monCtx, appCopy, policyRef, &policy)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Policy %s render error: %v", policy.Name, err))
			result.PolicyResults = append(result.PolicyResults, PolicyApplicationResult{
				Sequence:        policySequence,
				PolicyName:      policy.Name,
				PolicyNamespace: policy.Namespace,
				Priority:        policy.Spec.Priority,
				Enabled:         false,
				Applied:         false,
				Error:           err.Error(),
			})
			continue
		}

		// Apply the rendered policy
		var policyChanges *PolicyChanges
		monCtx, policyChanges, err = handler.applyRenderedPolicyResult(monCtx, appCopy, renderedResult, policySequence, policy.Spec.Priority)

		policyResult := PolicyApplicationResult{
			Sequence:        policySequence,
			PolicyName:      policy.Name,
			PolicyNamespace: policy.Namespace,
			Priority:        policy.Spec.Priority,
			Enabled:         renderedResult.Enabled,
			Applied:         renderedResult.Enabled && err == nil,
			SkipReason:      renderedResult.SkipReason,
		}

		if err != nil {
			policyResult.Error = err.Error()
			result.Errors = append(result.Errors, fmt.Sprintf("Policy %s application error: %v", policy.Name, err))
		} else if renderedResult.Enabled && policyChanges != nil {
			// Extract changes from policyChanges
			policyResult.SpecModified = policyChanges.SpecModified
			policyResult.AddedLabels = policyChanges.AddedLabels
			policyResult.AddedAnnotations = policyChanges.AddedAnnotations

			// Convert additional context from map to RawExtension
			if policyChanges.AdditionalContext != nil {
				contextBytes, err := json.Marshal(policyChanges.AdditionalContext)
				if err == nil {
					policyResult.AdditionalContext = &runtime.RawExtension{Raw: contextBytes}
				}
			}

			policySequence++
		}

		result.PolicyResults = append(result.PolicyResults, policyResult)
	}

	result.Application = appCopy
	return result, nil
}

// loadPolicyDefinition loads a PolicyDefinition from the cluster
func loadPolicyDefinition(ctx context.Context, cli client.Client, name, namespace string) (*v1beta1.PolicyDefinition, error) {
	policy := &v1beta1.PolicyDefinition{}
	err := cli.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, policy)
	if err != nil {
		return nil, err
	}
	return policy, nil
}

// discoverAndDeduplicateGlobalPolicies discovers global policies from both vela-system and app namespace
// and returns the deduplicated list (namespace policies win over vela-system)
func discoverAndDeduplicateGlobalPolicies(ctx monitorContext.Context, cli client.Client, appNamespace string) ([]v1beta1.PolicyDefinition, error) {
	var globalPolicies []v1beta1.PolicyDefinition

	// Discover from vela-system
	velaSystemPolicies, err := discoverGlobalPolicies(ctx, cli, oam.SystemDefinitionNamespace)
	if err != nil {
		return nil, err
	}

	// Discover from app namespace (if different)
	var namespacePolicies []v1beta1.PolicyDefinition
	if appNamespace != oam.SystemDefinitionNamespace {
		namespacePolicies, err = discoverGlobalPolicies(ctx, cli, appNamespace)
		if err != nil {
			// Non-fatal, continue with vela-system policies only
			namespacePolicies = []v1beta1.PolicyDefinition{}
		}
	}

	// Deduplicate: namespace policies win
	namespacePolicyNames := make(map[string]bool)
	for _, policy := range namespacePolicies {
		namespacePolicyNames[policy.Name] = true
	}

	// Add namespace policies first
	globalPolicies = append(globalPolicies, namespacePolicies...)

	// Add vela-system policies (skip if name exists in namespace)
	for _, policy := range velaSystemPolicies {
		if !namespacePolicyNames[policy.Name] {
			globalPolicies = append(globalPolicies, policy)
		}
	}

	return globalPolicies, nil
}
