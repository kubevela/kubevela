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

package v1beta1

import (
	"reflect"

	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/scheme"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

// Package type metadata.
const (
	Group   = common.Group
	Version = "v1beta1"
)

var (
	// SchemeGroupVersion is group version used to register these objects
	SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}

	// AddToScheme is a global function that registers this API group & version to a scheme
	AddToScheme = SchemeBuilder.AddToScheme
)

// ComponentDefinition type metadata.
var (
	ComponentDefinitionKind             = reflect.TypeOf(ComponentDefinition{}).Name()
	ComponentDefinitionGroupKind        = schema.GroupKind{Group: Group, Kind: ComponentDefinitionKind}.String()
	ComponentDefinitionKindAPIVersion   = ComponentDefinitionKind + "." + SchemeGroupVersion.String()
	ComponentDefinitionGroupVersionKind = SchemeGroupVersion.WithKind(ComponentDefinitionKind)
)

// WorkloadDefinition type metadata.
var (
	WorkloadDefinitionKind             = reflect.TypeOf(WorkloadDefinition{}).Name()
	WorkloadDefinitionGroupKind        = schema.GroupKind{Group: Group, Kind: WorkloadDefinitionKind}.String()
	WorkloadDefinitionKindAPIVersion   = WorkloadDefinitionKind + "." + SchemeGroupVersion.String()
	WorkloadDefinitionGroupVersionKind = SchemeGroupVersion.WithKind(WorkloadDefinitionKind)
)

// TraitDefinition type metadata.
var (
	TraitDefinitionKind             = reflect.TypeOf(TraitDefinition{}).Name()
	TraitDefinitionGroupKind        = schema.GroupKind{Group: Group, Kind: TraitDefinitionKind}.String()
	TraitDefinitionKindAPIVersion   = TraitDefinitionKind + "." + SchemeGroupVersion.String()
	TraitDefinitionGroupVersionKind = SchemeGroupVersion.WithKind(TraitDefinitionKind)
)

// PolicyDefinition type metadata.
var (
	PolicyDefinitionKind             = reflect.TypeOf(PolicyDefinition{}).Name()
	PolicyDefinitionGroupKind        = schema.GroupKind{Group: Group, Kind: PolicyDefinitionKind}.String()
	PolicyDefinitionKindAPIVersion   = PolicyDefinitionKind + "." + SchemeGroupVersion.String()
	PolicyDefinitionGroupVersionKind = SchemeGroupVersion.WithKind(PolicyDefinitionKind)
)

// WorkflowStepDefinition type metadata.
var (
	WorkflowStepDefinitionKind             = reflect.TypeOf(WorkflowStepDefinition{}).Name()
	WorkflowStepDefinitionGroupKind        = schema.GroupKind{Group: Group, Kind: WorkflowStepDefinitionKind}.String()
	WorkflowStepDefinitionKindAPIVersion   = WorkflowStepDefinitionKind + "." + SchemeGroupVersion.String()
	WorkflowStepDefinitionGroupVersionKind = SchemeGroupVersion.WithKind(WorkflowStepDefinitionKind)
)

// DefinitionRevision type metadata.
var (
	DefinitionRevisionKind             = reflect.TypeOf(DefinitionRevision{}).Name()
	DefinitionRevisionGroupKind        = schema.GroupKind{Group: Group, Kind: DefinitionRevisionKind}.String()
	DefinitionRevisionKindAPIVersion   = DefinitionRevisionKind + "." + SchemeGroupVersion.String()
	DefinitionRevisionGroupVersionKind = SchemeGroupVersion.WithKind(DefinitionRevisionKind)
)

// Application type metadata.
var (
	ApplicationKind            = reflect.TypeOf(Application{}).Name()
	ApplicationGroupKind       = schema.GroupKind{Group: Group, Kind: ApplicationKind}.String()
	ApplicationKindAPIVersion  = ApplicationKind + "." + SchemeGroupVersion.String()
	ApplicationKindVersionKind = SchemeGroupVersion.WithKind(ApplicationKind)
)

// ApplicationRevision type metadata
var (
	ApplicationRevisionKind             = reflect.TypeOf(ApplicationRevision{}).Name()
	ApplicationRevisionGroupKind        = schema.GroupKind{Group: Group, Kind: ApplicationRevisionKind}.String()
	ApplicationRevisionKindAPIVersion   = ApplicationRevisionKind + "." + SchemeGroupVersion.String()
	ApplicationRevisionGroupVersionKind = SchemeGroupVersion.WithKind(ApplicationRevisionKind)
)

// ScopeDefinition type metadata.
var (
	ScopeDefinitionKind             = reflect.TypeOf(ScopeDefinition{}).Name()
	ScopeDefinitionGroupKind        = schema.GroupKind{Group: Group, Kind: ScopeDefinitionKind}.String()
	ScopeDefinitionKindAPIVersion   = ScopeDefinitionKind + "." + SchemeGroupVersion.String()
	ScopeDefinitionGroupVersionKind = SchemeGroupVersion.WithKind(ScopeDefinitionKind)
)

// ResourceTracker type metadata.
var (
	ResourceTrackerKind            = reflect.TypeOf(ResourceTracker{}).Name()
	ResourceTrackerGroupKind       = schema.GroupKind{Group: Group, Kind: ResourceTrackerKind}.String()
	ResourceTrackerKindAPIVersion  = ResourceTrackerKind + "." + SchemeGroupVersion.String()
	ResourceTrackerKindVersionKind = SchemeGroupVersion.WithKind(ResourceTrackerKind)
)

func init() {
	SchemeBuilder.Register(&ComponentDefinition{}, &ComponentDefinitionList{})
	SchemeBuilder.Register(&WorkloadDefinition{}, &WorkloadDefinitionList{})
	SchemeBuilder.Register(&TraitDefinition{}, &TraitDefinitionList{})
	SchemeBuilder.Register(&PolicyDefinition{}, &PolicyDefinitionList{})
	SchemeBuilder.Register(&WorkflowStepDefinition{}, &WorkflowStepDefinitionList{})
	SchemeBuilder.Register(&DefinitionRevision{}, &DefinitionRevisionList{})
	SchemeBuilder.Register(&ScopeDefinition{}, &ScopeDefinitionList{})
	SchemeBuilder.Register(&Application{}, &ApplicationList{})
	SchemeBuilder.Register(&ApplicationRevision{}, &ApplicationRevisionList{})
	SchemeBuilder.Register(&ResourceTracker{}, &ResourceTrackerList{})
	_ = SchemeBuilder.AddToScheme(k8sscheme.Scheme)
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}
