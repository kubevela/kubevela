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

package v1alpha2

import (
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/types"
)

// A WorkloadDefinitionSpec defines the desired state of a WorkloadDefinition.
type WorkloadDefinitionSpec struct {
	// Reference to the CustomResourceDefinition that defines this workload kind.
	Reference common.DefinitionReference `json:"definitionRef"`

	// ChildResourceKinds are the list of GVK of the child resources this workload generates
	ChildResourceKinds []common.ChildResourceKind `json:"childResourceKinds,omitempty"`

	// RevisionLabel indicates which label for underlying resources(e.g. pods) of this workload
	// can be used by trait to create resource selectors(e.g. label selector for pods).
	// +optional
	RevisionLabel string `json:"revisionLabel,omitempty"`

	// PodSpecPath indicates where/if this workload has K8s podSpec field
	// if one workload has podSpec, trait can do lot's of assumption such as port, env, volume fields.
	// +optional
	PodSpecPath string `json:"podSpecPath,omitempty"`

	// Status defines the custom health policy and status message for workload
	// +optional
	Status *common.Status `json:"status,omitempty"`

	// Schematic defines the data format and template of the encapsulation of the workload
	// +optional
	Schematic *common.Schematic `json:"schematic,omitempty"`

	// Extension is used for extension needs by OAM platform builders
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Extension *runtime.RawExtension `json:"extension,omitempty"`
}

// WorkloadDefinitionStatus is the status of WorkloadDefinition
type WorkloadDefinitionStatus struct {
	runtimev1alpha1.ConditionedStatus `json:",inline"`
}

// +kubebuilder:object:root=true

// A WorkloadDefinition registers a kind of Kubernetes custom resource as a
// valid OAM workload kind by referencing its CustomResourceDefinition. The CRD
// is used to validate the schema of the workload when it is embedded in an OAM
// Component.
// +kubebuilder:resource:scope=Namespaced,categories={oam},shortName=workload
// +kubebuilder:printcolumn:name="DEFINITION-NAME",type=string,JSONPath=".spec.definitionRef.name"
type WorkloadDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkloadDefinitionSpec   `json:"spec,omitempty"`
	Status WorkloadDefinitionStatus `json:"status,omitempty"`
}

// SetConditions set condition for WorkloadDefinition
func (wd *WorkloadDefinition) SetConditions(c ...runtimev1alpha1.Condition) {
	wd.Status.SetConditions(c...)
}

// GetCondition gets condition from WorkloadDefinition
func (wd *WorkloadDefinition) GetCondition(conditionType runtimev1alpha1.ConditionType) runtimev1alpha1.Condition {
	return wd.Status.GetCondition(conditionType)
}

// +kubebuilder:object:root=true

// WorkloadDefinitionList contains a list of WorkloadDefinition.
type WorkloadDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadDefinition `json:"items"`
}

// A TraitDefinitionSpec defines the desired state of a TraitDefinition.
type TraitDefinitionSpec struct {
	// Reference to the CustomResourceDefinition that defines this trait kind.
	Reference common.DefinitionReference `json:"definitionRef,omitempty"`

	// Revision indicates whether a trait is aware of component revision
	// +optional
	RevisionEnabled bool `json:"revisionEnabled,omitempty"`

	// WorkloadRefPath indicates where/if a trait accepts a workloadRef object
	// +optional
	WorkloadRefPath string `json:"workloadRefPath,omitempty"`

	// PodDisruptive specifies whether using the trait will cause the pod to restart or not.
	// +optional
	PodDisruptive bool `json:"podDisruptive,omitempty"`

	// AppliesToWorkloads specifies the list of workload kinds this trait
	// applies to. Workload kinds are specified in kind.group/version format,
	// e.g. server.core.oam.dev/v1alpha2. Traits that omit this field apply to
	// all workload kinds.
	// +optional
	AppliesToWorkloads []string `json:"appliesToWorkloads,omitempty"`

	// ConflictsWith specifies the list of traits(CRD name, Definition name, CRD group)
	// which could not apply to the same workloads with this trait.
	// Traits that omit this field can work with any other traits.
	// Example rules:
	// "service" # Trait definition name
	// "services.k8s.io" # API resource/crd name
	// "*.networking.k8s.io" # API group
	// "labelSelector:foo=bar" # label selector
	// labelSelector format: https://pkg.go.dev/k8s.io/apimachinery/pkg/labels#Parse
	// +optional
	ConflictsWith []string `json:"conflictsWith,omitempty"`

	// Schematic defines the data format and template of the encapsulation of the trait
	// +optional
	Schematic *common.Schematic `json:"schematic,omitempty"`

	// Status defines the custom health policy and status message for trait
	// +optional
	Status *common.Status `json:"status,omitempty"`

	// Extension is used for extension needs by OAM platform builders
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Extension *runtime.RawExtension `json:"extension,omitempty"`
}

// TraitDefinitionStatus is the status of TraitDefinition
type TraitDefinitionStatus struct {
	// ConditionedStatus reflects the observed status of a resource
	runtimev1alpha1.ConditionedStatus `json:",inline"`
	// ConfigMapRef refer to a ConfigMap which contains OpenAPI V3 JSON schema of Component parameters.
	ConfigMapRef string `json:"configMapRef,omitempty"`
	// LatestRevision of the trait definition
	// +optional
	LatestRevision *common.Revision `json:"latestRevision,omitempty"`
}

// +kubebuilder:object:root=true

// A TraitDefinition registers a kind of Kubernetes custom resource as a valid
// OAM trait kind by referencing its CustomResourceDefinition. The CRD is used
// to validate the schema of the trait when it is embedded in an OAM
// ApplicationConfiguration.
// +kubebuilder:resource:scope=Namespaced,categories={oam},shortName=trait
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="APPLIES-TO",type=string,JSONPath=".spec.appliesToWorkloads"
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,JSONPath=".metadata.annotations.definition\\.oam\\.dev/description"
type TraitDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TraitDefinitionSpec   `json:"spec,omitempty"`
	Status TraitDefinitionStatus `json:"status,omitempty"`
}

// SetConditions set condition for TraitDefinition
func (td *TraitDefinition) SetConditions(c ...runtimev1alpha1.Condition) {
	td.Status.SetConditions(c...)
}

// GetCondition gets condition from TraitDefinition
func (td *TraitDefinition) GetCondition(conditionType runtimev1alpha1.ConditionType) runtimev1alpha1.Condition {
	return td.Status.GetCondition(conditionType)
}

// +kubebuilder:object:root=true

// TraitDefinitionList contains a list of TraitDefinition.
type TraitDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TraitDefinition `json:"items"`
}

// A ScopeDefinitionSpec defines the desired state of a ScopeDefinition.
type ScopeDefinitionSpec struct {
	// Reference to the CustomResourceDefinition that defines this scope kind.
	Reference common.DefinitionReference `json:"definitionRef"`

	// WorkloadRefsPath indicates if/where a scope accepts workloadRef objects
	WorkloadRefsPath string `json:"workloadRefsPath,omitempty"`

	// AllowComponentOverlap specifies whether an OAM component may exist in
	// multiple instances of this kind of scope.
	AllowComponentOverlap bool `json:"allowComponentOverlap"`

	// Extension is used for extension needs by OAM platform builders
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Extension *runtime.RawExtension `json:"extension,omitempty"`
}

// +kubebuilder:object:root=true

// A ScopeDefinition registers a kind of Kubernetes custom resource as a valid
// OAM scope kind by referencing its CustomResourceDefinition. The CRD is used
// to validate the schema of the scope when it is embedded in an OAM
// ApplicationConfiguration.
// +kubebuilder:printcolumn:JSONPath=".spec.definitionRef.name",name=DEFINITION-NAME,type=string
// +kubebuilder:resource:scope=Namespaced,categories={oam},shortName=scope
type ScopeDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ScopeDefinitionSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// ScopeDefinitionList contains a list of ScopeDefinition.
type ScopeDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScopeDefinition `json:"items"`
}

// A ComponentParameter defines a configurable parameter of a component.
type ComponentParameter struct {
	// Name of this parameter. OAM ApplicationConfigurations will specify
	// parameter values using this name.
	Name string `json:"name"`

	// FieldPaths specifies an array of fields within this Component's workload
	// that will be overwritten by the value of this parameter. The type of the
	// parameter (e.g. int, string) is inferred from the type of these fields;
	// All fields must be of the same type. Fields are specified as JSON field
	// paths without a leading dot, for example 'spec.replicas'.
	FieldPaths []string `json:"fieldPaths"`

	// +kubebuilder:default:=false
	// Required specifies whether or not a value for this parameter must be
	// supplied when authoring an ApplicationConfiguration.
	// +optional
	Required *bool `json:"required,omitempty"`

	// Description of this parameter.
	// +optional
	Description *string `json:"description,omitempty"`
}

// A ComponentSpec defines the desired state of a Component.
type ComponentSpec struct {
	// A Workload that will be created for each ApplicationConfiguration that
	// includes this Component. Workload is an instance of a workloadDefinition.
	// We either use the GVK info or a special "type" field in the workload to associate
	// the content of the workload with its workloadDefinition
	// +kubebuilder:validation:EmbeddedResource
	// +kubebuilder:pruning:PreserveUnknownFields
	Workload runtime.RawExtension `json:"workload"`

	// HelmRelease records a Helm release used by a Helm module workload.
	// +optional
	Helm *common.Helm `json:"helm,omitempty"`

	// Parameters exposed by this component. ApplicationConfigurations that
	// reference this component may specify values for these parameters, which
	// will in turn be injected into the embedded workload.
	// +optional
	Parameters []ComponentParameter `json:"parameters,omitempty"`
}

// A ComponentStatus represents the observed state of a Component.
type ComponentStatus struct {
	// The generation observed by the component controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration"`

	runtimev1alpha1.ConditionedStatus `json:",inline"`

	// LatestRevision of component
	// +optional
	LatestRevision *common.Revision `json:"latestRevision,omitempty"`

	// One Component should only be used by one AppConfig
}

// +kubebuilder:object:root=true

// A Component describes how an OAM workload kind may be instantiated.
// +kubebuilder:resource:categories={oam}
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".spec.workload.kind",name=WORKLOAD-KIND,type=string
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
type Component struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentSpec   `json:"spec,omitempty"`
	Status ComponentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComponentList contains a list of Component.
type ComponentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Component `json:"items"`
}

// A ComponentParameterValue specifies a value for a named parameter. The
// associated component must publish a parameter with this name.
type ComponentParameterValue struct {
	// Name of the component parameter to set.
	Name string `json:"name"`

	// Value to set.
	Value intstr.IntOrString `json:"value"`
}

// A ComponentTrait specifies a trait that should be applied to a component.
type ComponentTrait struct {
	// A Trait that will be created for the component
	// +kubebuilder:validation:EmbeddedResource
	// +kubebuilder:pruning:PreserveUnknownFields
	Trait runtime.RawExtension `json:"trait"`

	// DataOutputs specify the data output sources from this trait.
	// +optional
	DataOutputs []DataOutput `json:"dataOutputs,omitempty"`

	// DataInputs specify the data input sinks into this trait.
	// +optional
	DataInputs []DataInput `json:"dataInputs,omitempty"`
}

// A ComponentScope specifies a scope in which a component should exist.
type ComponentScope struct {
	// A ScopeReference must refer to an OAM scope resource.
	ScopeReference runtimev1alpha1.TypedReference `json:"scopeRef"`
}

// An ApplicationConfigurationComponent specifies a component of an
// ApplicationConfiguration. Each component is used to instantiate a workload.
type ApplicationConfigurationComponent struct {
	// ComponentName specifies a component whose latest revision will be bind
	// with ApplicationConfiguration. When the spec of the referenced component
	// changes, ApplicationConfiguration will automatically migrate all trait
	// affect from the prior revision to the new one. This is mutually exclusive
	// with RevisionName.
	// +optional
	ComponentName string `json:"componentName,omitempty"`

	// RevisionName of a specific component revision to which to bind
	// ApplicationConfiguration. This is mutually exclusive with componentName.
	// +optional
	RevisionName string `json:"revisionName,omitempty"`

	// DataOutputs specify the data output sources from this component.
	DataOutputs []DataOutput `json:"dataOutputs,omitempty"`

	// DataInputs specify the data input sinks into this component.
	DataInputs []DataInput `json:"dataInputs,omitempty"`

	// ParameterValues specify values for the the specified component's
	// parameters. Any parameter required by the component must be specified.
	// +optional
	ParameterValues []ComponentParameterValue `json:"parameterValues,omitempty"`

	// Traits of the specified component.
	// +optional
	Traits []ComponentTrait `json:"traits,omitempty"`

	// Scopes in which the specified component should exist.
	// +optional
	Scopes []ComponentScope `json:"scopes,omitempty"`
}

// An ApplicationConfigurationSpec defines the desired state of a
// ApplicationConfiguration.
type ApplicationConfigurationSpec struct {
	// Components of which this ApplicationConfiguration consists. Each
	// component will be used to instantiate a workload.
	Components []ApplicationConfigurationComponent `json:"components"`
}

// A TraitStatus represents the state of a trait.
type TraitStatus string

// A WorkloadTrait represents a trait associated with a workload and its status
type WorkloadTrait struct {
	// Status is a place holder for a customized controller to fill
	// if it needs a single place to summarize the status of the trait
	Status TraitStatus `json:"status,omitempty"`

	// Reference to a trait created by an ApplicationConfiguration.
	Reference runtimev1alpha1.TypedReference `json:"traitRef"`

	// Message will allow controller to leave some additional information for this trait
	Message string `json:"message,omitempty"`

	// AppliedGeneration indicates the generation observed by the appConfig controller.
	// The same field is also recorded in the annotations of traits.
	// A trait is possible to be deleted from cluster after created.
	// This field is useful to track the observed generation of traits after they are
	// deleted.
	AppliedGeneration int64 `json:"appliedGeneration,omitempty"`

	// DependencyUnsatisfied notify does the trait has dependency unsatisfied
	DependencyUnsatisfied bool `json:"dependencyUnsatisfied,omitempty"`
}

// A ScopeStatus represents the state of a scope.
type ScopeStatus string

// A WorkloadScope represents a scope associated with a workload and its status
type WorkloadScope struct {
	// Status is a place holder for a customized controller to fill
	// if it needs a single place to summarize the status of the scope
	Status ScopeStatus `json:"status,omitempty"`

	// Reference to a scope created by an ApplicationConfiguration.
	Reference runtimev1alpha1.TypedReference `json:"scopeRef"`
}

// A WorkloadStatus represents the status of a workload.
type WorkloadStatus struct {
	// Status is a place holder for a customized controller to fill
	// if it needs a single place to summarize the entire status of the workload
	Status string `json:"status,omitempty"`

	// ComponentName that produced this workload.
	ComponentName string `json:"componentName,omitempty"`

	// ComponentRevisionName of current component
	ComponentRevisionName string `json:"componentRevisionName,omitempty"`

	// DependencyUnsatisfied notify does the workload has dependency unsatisfied
	DependencyUnsatisfied bool `json:"dependencyUnsatisfied,omitempty"`

	// AppliedComponentRevision indicates the applied component revision name of this workload
	AppliedComponentRevision string `json:"appliedComponentRevision,omitempty"`

	// Reference to a workload created by an ApplicationConfiguration.
	Reference runtimev1alpha1.TypedReference `json:"workloadRef,omitempty"`

	// Traits associated with this workload.
	Traits []WorkloadTrait `json:"traits,omitempty"`

	// Scopes associated with this workload.
	Scopes []WorkloadScope `json:"scopes,omitempty"`
}

// HistoryWorkload contain the old component revision that are still running
type HistoryWorkload struct {
	// Revision of this workload
	Revision string `json:"revision,omitempty"`

	// Reference to running workload.
	Reference runtimev1alpha1.TypedReference `json:"workloadRef,omitempty"`
}

// A ApplicationStatus represents the state of the entire application.
type ApplicationStatus string

// An ApplicationConfigurationStatus represents the observed state of a
// ApplicationConfiguration.
type ApplicationConfigurationStatus struct {
	runtimev1alpha1.ConditionedStatus `json:",inline"`

	// Status is a place holder for a customized controller to fill
	// if it needs a single place to summarize the status of the entire application
	Status ApplicationStatus `json:"status,omitempty"`

	Dependency DependencyStatus `json:"dependency,omitempty"`

	// RollingStatus indicates what phase are we in the rollout phase
	RollingStatus types.RollingStatus `json:"rollingStatus,omitempty"`

	// Workloads created by this ApplicationConfiguration.
	Workloads []WorkloadStatus `json:"workloads,omitempty"`

	// The generation observed by the appConfig controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration"`

	// HistoryWorkloads will record history but still working revision workloads.
	HistoryWorkloads []HistoryWorkload `json:"historyWorkloads,omitempty"`
}

// DependencyStatus represents the observed state of the dependency of
// an ApplicationConfiguration.
type DependencyStatus struct {
	Unsatisfied []UnstaifiedDependency `json:"unsatisfied,omitempty"`
}

// UnstaifiedDependency describes unsatisfied dependency flow between
// one pair of objects.
type UnstaifiedDependency struct {
	Reason string               `json:"reason"`
	From   DependencyFromObject `json:"from"`
	To     DependencyToObject   `json:"to"`
}

// DependencyFromObject represents the object that dependency data comes from.
type DependencyFromObject struct {
	runtimev1alpha1.TypedReference `json:",inline"`
	FieldPath                      string `json:"fieldPath,omitempty"`
}

// DependencyToObject represents the object that dependency data goes to.
type DependencyToObject struct {
	runtimev1alpha1.TypedReference `json:",inline"`
	FieldPaths                     []string `json:"fieldPaths,omitempty"`
}

// +kubebuilder:object:root=true

// An ApplicationConfiguration represents an OAM application.
// +kubebuilder:resource:shortName=appconfig,categories={oam}
// +kubebuilder:subresource:status
type ApplicationConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationConfigurationSpec   `json:"spec,omitempty"`
	Status ApplicationConfigurationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApplicationConfigurationList contains a list of ApplicationConfiguration.
type ApplicationConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApplicationConfiguration `json:"items"`
}

// DataOutput specifies a data output source from an object.
type DataOutput struct {
	// Name is the unique name of a DataOutput in an ApplicationConfiguration.
	Name string `json:"name,omitempty"`

	// FieldPath refers to the value of an object's field.
	FieldPath string `json:"fieldPath,omitempty"`

	// Conditions specify the conditions that should be satisfied before emitting a data output.
	// Different conditions are AND-ed together.
	// If no conditions is specified, it is by default to check output value not empty.
	// +optional
	Conditions []ConditionRequirement `json:"conditions,omitempty"`
	// OutputStore specifies the object used to store intermediate data generated by Operations
	OutputStore StoreReference `json:"outputStore,omitempty"`
}

// StoreReference specifies the referenced object in DataOutput or DataInput
type StoreReference struct {
	runtimev1alpha1.TypedReference `json:",inline"`
	// Operations specify the data processing operations
	Operations []DataOperation `json:"operations,omitempty"`
}

// DataOperation defines the specific operation for data
type DataOperation struct {
	// Type specifies the type of DataOperation
	Type string `json:"type"`
	// Operator specifies the operation under this DataOperation type
	Operator DataOperator `json:"op"`
	// ToFieldPath refers to the value of an object's field
	ToFieldPath string `json:"toFieldPath"`
	// ToDataPath refers to the value of an object's specfied by ToDataPath. For example the ToDataPath "redis" specifies "redis info" in '{"redis":"redis info"}'
	ToDataPath string `json:"toDataPath,omitempty"`
	// +optional
	// Value specifies an expected value
	// This is mutually exclusive with ValueFrom
	Value string `json:"value,omitempty"`
	// +optional
	// ValueFrom specifies expected value from object such as workload and trait
	// This is mutually exclusive with Value
	ValueFrom  ValueFrom              `json:"valueFrom,omitempty"`
	Conditions []ConditionRequirement `json:"conditions,omitempty"`
}

// DataOperator defines the type of Operator in DataOperation
type DataOperator string

const (
	// AddOperator specifies the add operation for data passing
	AddOperator DataOperator = "add"
	// DeleteOperator specifies the delete operation for data passing
	DeleteOperator DataOperator = "delete"
	// ReplaceOperator specifies the replace operation for data passing
	ReplaceOperator DataOperator = "replace"
)

// DataInput specifies a data input sink to an object.
// If input is array, it will be appended to the target field paths.
type DataInput struct {
	// ValueFrom specifies the value source.
	ValueFrom DataInputValueFrom `json:"valueFrom,omitempty"`

	// ToFieldPaths specifies the field paths of an object to fill passed value.
	ToFieldPaths []string `json:"toFieldPaths,omitempty"`

	// StrategyMergeKeys specifies the merge key if the toFieldPaths target is an array.
	// The StrategyMergeKeys is optional, by default, if the toFieldPaths target is an array, we will append.
	// If StrategyMergeKeys specified, we will check the key in the target array.
	// If any key exist, do update; if no key exist, append.
	StrategyMergeKeys []string `json:"strategyMergeKeys,omitempty"`

	// When the Conditions is satified, ToFieldPaths will be filled with passed value
	Conditions []ConditionRequirement `json:"conditions,omitempty"`

	// InputStore specifies the object used to read intermediate data genereted by DataOutput
	InputStore StoreReference `json:"inputStore,omitempty"`
}

// DataInputValueFrom specifies the value source for a data input.
type DataInputValueFrom struct {
	// DataOutputName matches a name of a DataOutput in the same AppConfig.
	DataOutputName string `json:"dataOutputName"`
}

// ConditionRequirement specifies the requirement to match a value.
type ConditionRequirement struct {
	Operator ConditionOperator `json:"op"`

	// +optional
	// Value specifies an expected value
	// This is mutually exclusive with ValueFrom
	Value string `json:"value,omitempty"`
	// +optional
	// ValueFrom specifies expected value from AppConfig
	// This is mutually exclusive with Value
	ValueFrom ValueFrom `json:"valueFrom,omitempty"`

	// +optional
	// FieldPath specifies got value from workload/trait object
	FieldPath string `json:"fieldPath,omitempty"`
}

// ValueFrom gets value from AppConfig object by specifying a path
type ValueFrom struct {
	FieldPath string `json:"fieldPath"`
}

// ConditionOperator specifies the operator to match a value.
type ConditionOperator string

const (
	// ConditionEqual indicates equal to given value
	ConditionEqual ConditionOperator = "eq"
	// ConditionNotEqual indicates not equal to given value
	ConditionNotEqual ConditionOperator = "notEq"
	// ConditionNotEmpty indicates given value not empty
	ConditionNotEmpty ConditionOperator = "notEmpty"
)
