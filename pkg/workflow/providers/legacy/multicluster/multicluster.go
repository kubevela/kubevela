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

package multicluster

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/pkg/errors"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	pkgpolicy "github.com/oam-dev/kubevela/pkg/policy"
	"github.com/oam-dev/kubevela/pkg/policy/envbinding"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

// Inputs is the inputs for multi cluster
type Inputs[T any] struct {
	Inputs T `json:"inputs"`
}

// Outputs is the outputs for multi cluster
type Outputs[T any] struct {
	Outputs T `json:"outputs"`
}

// PlacementDecisionVars is the vars for make placement decisions
type PlacementDecisionVars struct {
	PolicyName string                 `json:"policyName"`
	EnvName    string                 `json:"envName"`
	Placement  *v1alpha1.EnvPlacement `json:"placement,omitempty"`
}

// PlacementDecisionResult is the result for make placement decisions
type PlacementDecisionResult struct {
	Decisions []v1alpha1.PlacementDecision `json:"decisions"`
}

// PlacementDecisionParams is the parameter for make placement decisions
type PlacementDecisionParams = oamprovidertypes.OAMParams[Inputs[PlacementDecisionVars]]

// PlacementDecisionReturns is the return value for make placement decisions
type PlacementDecisionReturns = Outputs[PlacementDecisionResult]

// MakePlacementDecisions ...
// Deprecated
func MakePlacementDecisions(ctx context.Context, params *PlacementDecisionParams) (*PlacementDecisionReturns, error) {
	policy := params.Params.Inputs.PolicyName
	if policy == "" {
		return nil, fmt.Errorf("empty policy name")
	}
	env := params.Params.Inputs.EnvName
	if env == "" {
		return nil, fmt.Errorf("empty env name")
	}
	placement := params.Params.Inputs.Placement
	if placement == nil {
		return nil, fmt.Errorf("empty placement for policy %s in env %s", policy, env)
	}

	var namespace, clusterName string
	// check if namespace selector is valid
	if placement.NamespaceSelector != nil {
		if len(placement.NamespaceSelector.Labels) != 0 {
			return nil, fmt.Errorf("invalid env %s: namespace selector in cluster-gateway does not support label selector for now", env)
		}
		namespace = placement.NamespaceSelector.Name
	}
	// check if cluster selector is valid
	if placement.ClusterSelector != nil {
		if len(placement.ClusterSelector.Labels) != 0 {
			return nil, fmt.Errorf("invalid env %s: cluster selector does not support label selector for now", env)
		}
		clusterName = placement.ClusterSelector.Name
	}
	// set fallback cluster
	if clusterName == "" {
		clusterName = multicluster.ClusterLocalName
	}
	// check if target cluster exists
	if clusterName != multicluster.ClusterLocalName {
		if _, err := multicluster.GetVirtualCluster(ctx, params.KubeClient, clusterName); err != nil {
			return nil, errors.Wrapf(err, "failed to get cluster %s for env %s", clusterName, env)
		}
	}
	// write result back
	decisions := []v1alpha1.PlacementDecision{{
		Cluster:   clusterName,
		Namespace: namespace,
	}}
	if err := envbinding.WritePlacementDecisions(params.App, policy, env, decisions); err != nil {
		return nil, err
	}
	return &PlacementDecisionReturns{Outputs: PlacementDecisionResult{Decisions: decisions}}, nil
}

// ApplicationVars is the vars for patching application
type ApplicationVars struct {
	EnvName  string                `json:"envName"`
	Patch    *v1alpha1.EnvPatch    `json:"patch,omitempty"`
	Selector *v1alpha1.EnvSelector `json:"selector,omitempty"`
}

// ApplicationParams is the parameter for patch application
type ApplicationParams = oamprovidertypes.OAMParams[Inputs[ApplicationVars]]

// PatchApplication ...
// Deprecated
func PatchApplication(_ context.Context, params *ApplicationParams) (*Outputs[*v1beta1.Application], error) {
	env := params.Params.Inputs.EnvName
	if env == "" {
		return nil, fmt.Errorf("empty env name")
	}
	patch := params.Params.Inputs.Patch
	selector := params.Params.Inputs.Selector

	newApp, err := envbinding.PatchApplication(params.App, patch, selector)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to patch app for env %s", env)
	}
	return &Outputs[*v1beta1.Application]{Outputs: newApp}, nil
}

// ClusterParams is the parameter for list clusters
type ClusterParams struct {
	Clusters []string `json:"clusters"`
}

// ClusterReturns is the return value for list clusters
type ClusterReturns = Outputs[ClusterParams]

// ListClusters lists clusters
func ListClusters(ctx context.Context, params *oamprovidertypes.OAMParams[any]) (*ClusterReturns, error) {
	secrets, err := multicluster.ListExistingClusterSecrets(ctx, params.KubeClient)
	if err != nil {
		return nil, err
	}
	var clusters []string
	for _, secret := range secrets {
		clusters = append(clusters, secret.Name)
	}
	return &ClusterReturns{Outputs: ClusterParams{Clusters: clusters}}, nil
}

// DeployParams is the parameter for deploy
type DeployParams = oamprovidertypes.OAMParams[DeployParameter]

// Deploy deploys the application
func Deploy(ctx context.Context, params *DeployParams) (*any, error) {
	if params.Params.Parallelism <= 0 {
		return nil, errors.Errorf("parallelism cannot be smaller than 1")
	}
	executor := NewDeployWorkflowStepExecutor(params.KubeClient, params.Appfile, params.ComponentApply, params.ComponentHealthCheck, params.WorkloadRender, params.Params)
	healthy, reason, err := executor.Deploy(ctx)
	if err != nil {
		return nil, err
	}
	if !healthy {
		params.Action.Wait(reason)
	}
	return nil, nil
}

// PoliciesVars is the vars for getting placements from topology policies
type PoliciesVars struct {
	Policies []string `json:"policies"`
}

// PoliciesResult is the result for getting placements from topology policies
type PoliciesResult struct {
	Placements []v1alpha1.PlacementDecision `json:"placements"`
}

// PoliciesParams is the params for getting placements from topology policies
type PoliciesParams = oamprovidertypes.OAMParams[PoliciesVars]

// GetPlacementsFromTopologyPolicies gets placements from topology policies
func GetPlacementsFromTopologyPolicies(ctx context.Context, params *PoliciesParams) (*PoliciesResult, error) {
	policyNames := params.Params.Policies
	policies, err := selectPolicies(params.Appfile.Policies, policyNames)
	if err != nil {
		return nil, err
	}
	placements, err := pkgpolicy.GetPlacementsFromTopologyPolicies(ctx, params.KubeClient, params.Appfile.Namespace, policies, true)
	if err != nil {
		return nil, err
	}
	return &PoliciesResult{Placements: placements}, nil
}

//go:embed multicluster.cue
var template string

// GetTemplate returns the cue template.
func GetTemplate() string {
	return template
}

// GetProviders returns the cue providers.
func GetProviders() map[string]cuexruntime.ProviderFn {
	return map[string]cuexruntime.ProviderFn{
		"make-placement-decisions":              oamprovidertypes.OAMGenericProviderFn[Inputs[PlacementDecisionVars], Outputs[PlacementDecisionResult]](MakePlacementDecisions),
		"patch-application":                     oamprovidertypes.OAMGenericProviderFn[Inputs[ApplicationVars], Outputs[*v1beta1.Application]](PatchApplication),
		"list-clusters":                         oamprovidertypes.OAMGenericProviderFn[any, ClusterReturns](ListClusters),
		"get-placements-from-topology-policies": oamprovidertypes.OAMGenericProviderFn[PoliciesVars, PoliciesResult](GetPlacementsFromTopologyPolicies),
		"deploy":                                oamprovidertypes.OAMGenericProviderFn[DeployParameter, any](Deploy),
	}
}
