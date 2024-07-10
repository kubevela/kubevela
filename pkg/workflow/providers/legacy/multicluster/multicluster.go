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
	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	pkgpolicy "github.com/oam-dev/kubevela/pkg/policy"
	"github.com/oam-dev/kubevela/pkg/policy/envbinding"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/legacy/types"
)

type MultiClusterInputs[T any] struct {
	Inputs T `json:"inputs"`
}

type MultiClusterOutputs[T any] struct {
	Outputs T `json:"outputs"`
}

type PlacementDecisionVars struct {
	PolicyName string                 `json:"policyName"`
	EnvName    string                 `json:"envName"`
	Placement  *v1alpha1.EnvPlacement `json:"placement,omitempty"`
}

type PlacementDecisionResult struct {
	Decisions []v1alpha1.PlacementDecision `json:"decisions"`
}

type PlacementDecisionParams = oamprovidertypes.OAMParams[MultiClusterInputs[PlacementDecisionVars]]

type PlacementDecisionReturns = MultiClusterOutputs[PlacementDecisionResult]

// MakePlacementDecisions
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
		if _, err := multicluster.GetVirtualCluster(ctx, singleton.KubeClient.Get(), clusterName); err != nil {
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

type ApplicationVars struct {
	EnvName  string                `json:"envName"`
	Patch    *v1alpha1.EnvPatch    `json:"patch,omitempty"`
	Selector *v1alpha1.EnvSelector `json:"selector,omitempty"`
}

type ApplicationParams = oamprovidertypes.OAMParams[MultiClusterInputs[ApplicationVars]]

// PatchApplication
// Deprecated
func PatchApplication(ctx context.Context, params *ApplicationParams) (*MultiClusterOutputs[*v1beta1.Application], error) {
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
	return &MultiClusterOutputs[*v1beta1.Application]{Outputs: newApp}, nil
}

type ClusterParams struct {
	Clusters []string `json:"clusters"`
}

type ClusterReturns = MultiClusterOutputs[ClusterParams]

func ListClusters(ctx context.Context, params *oamprovidertypes.OAMParams[any]) (*ClusterReturns, error) {
	secrets, err := multicluster.ListExistingClusterSecrets(ctx, singleton.KubeClient.Get())
	if err != nil {
		return nil, err
	}
	var clusters []string
	for _, secret := range secrets {
		clusters = append(clusters, secret.Name)
	}
	return &ClusterReturns{Outputs: ClusterParams{Clusters: clusters}}, nil
}

type DeployParams = oamprovidertypes.OAMParams[DeployParameter]

func Deploy(ctx context.Context, params *DeployParams) (*any, error) {
	if params.Params.Parallelism <= 0 {
		return nil, errors.Errorf("parallelism cannot be smaller than 1")
	}
	executor := NewDeployWorkflowStepExecutor(singleton.KubeClient.Get(), params.Appfile, params.ComponentApply, params.ComponentHealthCheck, params.WorkloadRender, params.Params)
	healthy, reason, err := executor.Deploy(ctx)
	if err != nil {
		return nil, err
	}
	if !healthy {
		params.Action.Wait(reason)
	}
	return nil, nil
}

type PoliciesVars struct {
	Policies []string `json:"policies"`
}

type PoliciesResult struct {
	Placements []v1alpha1.PlacementDecision `json:"placements"`
}

type PoliciesParams = oamprovidertypes.OAMParams[PoliciesVars]

func GetPlacementsFromTopologyPolicies(ctx context.Context, params *PoliciesParams) (*PoliciesResult, error) {
	policyNames := params.Params.Policies
	policies, err := selectPolicies(params.Appfile.Policies, policyNames)
	if err != nil {
		return nil, err
	}
	placements, err := pkgpolicy.GetPlacementsFromTopologyPolicies(ctx, singleton.KubeClient.Get(), params.Appfile.Namespace, policies, true)
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
		"make-placement-decisions":              oamprovidertypes.OAMGenericProviderFn[MultiClusterInputs[PlacementDecisionVars], MultiClusterOutputs[PlacementDecisionResult]](MakePlacementDecisions),
		"patch-application":                     oamprovidertypes.OAMGenericProviderFn[MultiClusterInputs[ApplicationVars], MultiClusterOutputs[*v1beta1.Application]](PatchApplication),
		"list-clusters":                         oamprovidertypes.OAMGenericProviderFn[any, ClusterReturns](ListClusters),
		"get-placements-from-topology-policies": oamprovidertypes.OAMGenericProviderFn[PoliciesVars, PoliciesResult](GetPlacementsFromTopologyPolicies),
		"deploy":                                oamprovidertypes.OAMGenericProviderFn[DeployParameter, any](Deploy),
	}
}
