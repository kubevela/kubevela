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
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

const (
	// EnvBindingPolicyType refers to the type of EnvBinding
	EnvBindingPolicyType = "env-binding"
)

// EnvTraitPatch is the patch to trait
type EnvTraitPatch struct {
	Type       string                `json:"type"`
	Properties *runtime.RawExtension `json:"properties,omitempty"`
	Disable    bool                  `json:"disable,omitempty"`
}

// ToApplicationTrait convert EnvTraitPatch into ApplicationTrait
func (in *EnvTraitPatch) ToApplicationTrait() *common.ApplicationTrait {
	out := &common.ApplicationTrait{Type: in.Type}
	if in.Properties != nil {
		out.Properties = in.Properties.DeepCopy()
	}
	return out
}

// EnvComponentPatch is the patch to component
type EnvComponentPatch struct {
	Name             string                `json:"name"`
	Type             string                `json:"type"`
	Properties       *runtime.RawExtension `json:"properties,omitempty"`
	Traits           []EnvTraitPatch       `json:"traits,omitempty"`
	ExternalRevision string                `json:"externalRevision,omitempty"`
}

// ToApplicationComponent convert EnvComponentPatch into ApplicationComponent
func (in *EnvComponentPatch) ToApplicationComponent() *common.ApplicationComponent {
	out := &common.ApplicationComponent{
		Name: in.Name,
		Type: in.Type,
	}
	if in.Properties != nil {
		out.Properties = in.Properties.DeepCopy()
	}
	if in.Traits != nil {
		for _, trait := range in.Traits {
			if !trait.Disable {
				out.Traits = append(out.Traits, *trait.ToApplicationTrait())
			}
		}
	}
	return out
}

// EnvPatch specify the parameter configuration for different environments
type EnvPatch struct {
	Components []EnvComponentPatch `json:"components,omitempty"`
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
// Deprecated
type EnvConfig struct {
	Name      string       `json:"name"`
	Placement EnvPlacement `json:"placement,omitempty"`
	Selector  *EnvSelector `json:"selector,omitempty"`
	Patch     EnvPatch     `json:"patch,omitempty"`
}

// EnvBindingSpec defines a list of envs
// Deprecated This spec is deprecated and replaced by Topology/Override Policy
type EnvBindingSpec struct {
	Envs []EnvConfig `json:"envs"`
}

// PlacementDecision describes the placement of one application instance
type PlacementDecision struct {
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
}

// String encode placement decision
func (in PlacementDecision) String() string {
	if in.Namespace == "" {
		return in.Cluster
	}
	return in.Cluster + "/" + in.Namespace
}

// EnvStatus records the status of one env
// Deprecated
type EnvStatus struct {
	Env        string              `json:"env"`
	Placements []PlacementDecision `json:"placements"`
}

// ClusterConnection records the connection with clusters and the last active app revision when they are active (still be used)
// Deprecated
type ClusterConnection struct {
	ClusterName        string `json:"clusterName"`
	LastActiveRevision string `json:"lastActiveRevision"`
}

// EnvBindingStatus records the status of all env
// Deprecated
type EnvBindingStatus struct {
	Envs               []EnvStatus         `json:"envs"`
	ClusterConnections []ClusterConnection `json:"clusterConnections"`
}
