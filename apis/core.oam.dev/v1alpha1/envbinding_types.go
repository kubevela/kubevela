/*
 Copyright 2021. The KubeVela Authors.

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

package v1alpha1

import (
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

const (
	// EnvBindingPolicyType refers to the type of EnvBinding
	EnvBindingPolicyType = "env-binding"
)

// EnvPatch specify the parameter configuration for different environments
type EnvPatch struct {
	Components []common.ApplicationComponent `json:"components"`
}

// NamespaceSelector defines the rules to select a Namespace resource.
// Either name or labels is needed.
type NamespaceSelector struct {
	// Name is the name of the namespace.
	Name string `json:"name,omitempty"`
	// Labels defines the label selector to select the namespace.
	Labels map[string]string `json:"labels,omitempty"`
}

// EnvPlacement defines the placement rules for an app.
type EnvPlacement struct {
	ClusterSelector   *common.ClusterSelector `json:"clusterSelector,omitempty"`
	NamespaceSelector *NamespaceSelector      `json:"namespaceSelector,omitempty"`
}

// EnvSelector defines which components should this env contains
type EnvSelector struct {
	Components []string `json:"components,omitempty"`
}

// EnvConfig is the configuration for different environments.
type EnvConfig struct {
	Name      string       `json:"name"`
	Placement EnvPlacement `json:"placement,omitempty"`
	Selector  *EnvSelector `json:"selector,omitempty"`
	Patch     EnvPatch     `json:"patch"`
}

// EnvBindingSpec defines a list of envs
type EnvBindingSpec struct {
	Envs []EnvConfig `json:"envs"`
}

// PlacementDecision describes the placement of one application instance
type PlacementDecision struct {
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
}

// EnvStatus records the status of one env
type EnvStatus struct {
	Env        string              `json:"env"`
	Placements []PlacementDecision `json:"placements"`
}

// EnvBindingStatus records the status of all env
type EnvBindingStatus struct {
	Envs []EnvStatus `json:"envs"`
}
