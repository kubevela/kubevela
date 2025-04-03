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

	"github.com/pkg/errors"

	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	pkgpolicy "github.com/oam-dev/kubevela/pkg/policy"
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

// ApplicationVars is the vars for patching application
type ApplicationVars struct {
	EnvName  string                `json:"envName"`
	Patch    *v1alpha1.EnvPatch    `json:"patch,omitempty"`
	Selector *v1alpha1.EnvSelector `json:"selector,omitempty"`
}

// ApplicationParams is the parameter for patch application
type ApplicationParams = oamprovidertypes.OAMParams[Inputs[ApplicationVars]]

// ClusterParams is the parameter for list clusters
type ClusterParams struct {
	Clusters []string `json:"clusters"`
}

// ClusterReturns is the return value for list clusters
type ClusterReturns = oamprovidertypes.Returns[Outputs[ClusterParams]]

// ListClusters lists clusters
func ListClusters(ctx context.Context, params *oamprovidertypes.Params[any]) (*ClusterReturns, error) {
	secrets, err := multicluster.ListExistingClusterSecrets(ctx, params.KubeClient)
	if err != nil {
		return nil, err
	}
	var clusters []string
	for _, secret := range secrets {
		clusters = append(clusters, secret.Name)
	}
	return &ClusterReturns{Returns: Outputs[ClusterParams]{Outputs: ClusterParams{Clusters: clusters}}}, nil
}

// DeployParams is the parameter for deploy
type DeployParams = oamprovidertypes.Params[DeployParameter]

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
type PoliciesParams = oamprovidertypes.Params[PoliciesVars]

// PoliciesReturns is the return value for getting placements from topology policies
type PoliciesReturns = oamprovidertypes.Returns[PoliciesResult]

// GetPlacementsFromTopologyPolicies gets placements from topology policies
func GetPlacementsFromTopologyPolicies(ctx context.Context, params *PoliciesParams) (*PoliciesReturns, error) {
	policyNames := params.Params.Policies
	policies, err := selectPolicies(params.Appfile.Policies, policyNames)
	if err != nil {
		return nil, err
	}
	placements, err := pkgpolicy.GetPlacementsFromTopologyPolicies(ctx, params.KubeClient, params.Appfile.Namespace, policies, true)
	if err != nil {
		return nil, err
	}
	return &PoliciesReturns{Returns: PoliciesResult{Placements: placements}}, nil
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
		"list-clusters":                         oamprovidertypes.GenericProviderFn[any, ClusterReturns](ListClusters),
		"get-placements-from-topology-policies": oamprovidertypes.GenericProviderFn[PoliciesVars, PoliciesReturns](GetPlacementsFromTopologyPolicies),
		"deploy":                                oamprovidertypes.GenericProviderFn[DeployParameter, any](Deploy),
	}
}
