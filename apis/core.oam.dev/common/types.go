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

package common

import (
	"encoding/json"
	"errors"

	"github.com/oam-dev/terraform-controller/api/v1beta2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// Kube defines the encapsulation in raw Kubernetes resource format
type Kube struct {
	// Template defines the raw Kubernetes resource
	// +kubebuilder:pruning:PreserveUnknownFields
	Template runtime.RawExtension `json:"template"`

	// Parameters defines configurable parameters
	Parameters []KubeParameter `json:"parameters,omitempty"`
}

// ParameterValueType refers to a data type of parameter
type ParameterValueType string

// data types of parameter value
const (
	StringType  ParameterValueType = "string"
	NumberType  ParameterValueType = "number"
	BooleanType ParameterValueType = "boolean"
)

// A KubeParameter defines a configurable parameter of a component.
type KubeParameter struct {
	// Name of this parameter
	Name string `json:"name"`

	// +kubebuilder:validation:Enum:=string;number;boolean
	// ValueType indicates the type of the parameter value, and
	// only supports basic data types: string, number, boolean.
	ValueType ParameterValueType `json:"type"`

	// FieldPaths specifies an array of fields within this workload that will be
	// overwritten by the value of this parameter. 	All fields must be of the
	// same type. Fields are specified as JSON field paths without a leading
	// dot, for example 'spec.replicas'.
	FieldPaths []string `json:"fieldPaths"`

	// +kubebuilder:default:=false
	// Required specifies whether or not a value for this parameter must be
	// supplied when authoring an Application.
	Required *bool `json:"required,omitempty"`

	// Description of this parameter.
	Description *string `json:"description,omitempty"`
}

// CUE defines the encapsulation in CUE format
type CUE struct {
	// Template defines the abstraction template data of the capability, it will replace the old CUE template in extension field.
	// Template is a required field if CUE is defined in Capability Definition.
	Template string `json:"template"`
}

// Schematic defines the encapsulation of this capability(workload/trait/scope),
// the encapsulation can be defined in different ways, e.g. CUE/HCL(terraform)/KUBE(K8s Object)/HELM, etc...
type Schematic struct {
	KUBE *Kube `json:"kube,omitempty"`

	CUE *CUE `json:"cue,omitempty"`

	HELM *Helm `json:"helm,omitempty"`

	Terraform *Terraform `json:"terraform,omitempty"`
}

// A Helm represents resources used by a Helm module
type Helm struct {
	// Release records a Helm release used by a Helm module workload.
	// +kubebuilder:pruning:PreserveUnknownFields
	Release runtime.RawExtension `json:"release"`

	// HelmRelease records a Helm repository used by a Helm module workload.
	// +kubebuilder:pruning:PreserveUnknownFields
	Repository runtime.RawExtension `json:"repository"`
}

// Terraform is the struct to describe cloud resources managed by Hashicorp Terraform
type Terraform struct {
	// Configuration is Terraform Configuration
	Configuration string `json:"configuration"`

	// Type specifies which Terraform configuration it is, HCL or JSON syntax
	// +kubebuilder:default:=hcl
	// +kubebuilder:validation:Enum:=hcl;json;remote
	Type string `json:"type,omitempty"`

	// Path is the sub-directory of remote git repository. It's valid when remote is set
	Path string `json:"path,omitempty"`

	v1beta2.BaseConfigurationSpec `json:",inline"`
}

// A WorkloadTypeDescriptor refer to a Workload Type
type WorkloadTypeDescriptor struct {
	// Type ref to a WorkloadDefinition via name
	Type string `json:"type,omitempty"`
	// Definition mutually exclusive to workload.type, a embedded WorkloadDefinition
	Definition WorkloadGVK `json:"definition,omitempty"`
}

// WorkloadGVK refer to a Workload Type
type WorkloadGVK struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// A DefinitionReference refers to a CustomResourceDefinition by name.
type DefinitionReference struct {
	// Name of the referenced CustomResourceDefinition.
	Name string `json:"name"`

	// Version indicate which version should be used if CRD has multiple versions
	// by default it will use the first one if not specified
	Version string `json:"version,omitempty"`
}

// A ChildResourceKind defines a child Kubernetes resource kind with a selector
type ChildResourceKind struct {
	// APIVersion of the child resource
	APIVersion string `json:"apiVersion"`

	// Kind of the child resource
	Kind string `json:"kind"`

	// Selector to select the child resources that the workload wants to expose to traits
	Selector map[string]string `json:"selector,omitempty"`
}

// Status defines the loop back status of the abstraction by using CUE template
type Status struct {
	// CustomStatus defines the custom status message that could display to user
	// +optional
	CustomStatus string `json:"customStatus,omitempty"`
	// HealthPolicy defines the health check policy for the abstraction
	// +optional
	HealthPolicy string `json:"healthPolicy,omitempty"`
}

// ApplicationPhase is a label for the condition of an application at the current time
type ApplicationPhase string

const (
	// ApplicationRollingOut means the app is in the middle of rolling out
	ApplicationRollingOut ApplicationPhase = "rollingOut"
	// ApplicationStarting means the app is preparing for reconcile
	ApplicationStarting ApplicationPhase = "starting"
	// ApplicationRendering means the app is rendering
	ApplicationRendering ApplicationPhase = "rendering"
	// ApplicationPolicyGenerating means the app is generating policies
	ApplicationPolicyGenerating ApplicationPhase = "generatingPolicy"
	// ApplicationRunningWorkflow means the app is running workflow
	ApplicationRunningWorkflow ApplicationPhase = "runningWorkflow"
	// ApplicationWorkflowSuspending means the app's workflow is suspending
	ApplicationWorkflowSuspending ApplicationPhase = "workflowSuspending"
	// ApplicationWorkflowTerminated means the app's workflow is terminated
	ApplicationWorkflowTerminated ApplicationPhase = "workflowTerminated"
	// ApplicationWorkflowFinished means the app's workflow is finished
	ApplicationWorkflowFinished ApplicationPhase = "workflowFinished"
	// ApplicationRunning means the app finished rendering and applied result to the cluster
	ApplicationRunning ApplicationPhase = "running"
	// ApplicationUnhealthy means the app finished rendering and applied result to the cluster, but still unhealthy
	ApplicationUnhealthy ApplicationPhase = "unhealthy"
	// ApplicationDeleting means application is being deleted
	ApplicationDeleting ApplicationPhase = "deleting"
)

// WorkflowState is a string that mark the workflow state
type WorkflowState string

const (
	// WorkflowStateInitializing means the workflow is in initial state
	WorkflowStateInitializing WorkflowState = "initializing"
	// WorkflowStateTerminated means workflow is terminated manually, and it won't be started unless the spec changed.
	WorkflowStateTerminated WorkflowState = "terminated"
	// WorkflowStateSuspended means workflow is suspended manually, and it can be resumed.
	WorkflowStateSuspended WorkflowState = "suspended"
	// WorkflowStateSucceeded means workflow is running successfully, all steps finished.
	WorkflowStateSucceeded WorkflowState = "Succeeded"
	// WorkflowStateFinished means workflow is end.
	WorkflowStateFinished WorkflowState = "finished"
	// WorkflowStateExecuting means workflow is still running or waiting some steps.
	WorkflowStateExecuting WorkflowState = "executing"
	// WorkflowStateSkipping means it will skip this reconcile and let next reconcile to handle it.
	WorkflowStateSkipping WorkflowState = "skipping"
)

// ApplicationComponentStatus record the health status of App component
type ApplicationComponentStatus struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Cluster   string `json:"cluster,omitempty"`
	Env       string `json:"env,omitempty"`
	// WorkloadDefinition is the definition of a WorkloadDefinition, such as deployments/apps.v1
	WorkloadDefinition WorkloadGVK              `json:"workloadDefinition,omitempty"`
	Healthy            bool                     `json:"healthy"`
	Message            string                   `json:"message,omitempty"`
	Traits             []ApplicationTraitStatus `json:"traits,omitempty"`
	Scopes             []corev1.ObjectReference `json:"scopes,omitempty"`
}

// ApplicationTraitStatus records the trait health status
type ApplicationTraitStatus struct {
	Type    string `json:"type"`
	Healthy bool   `json:"healthy"`
	Message string `json:"message,omitempty"`
}

// Revision has name and revision number
type Revision struct {
	Name     string `json:"name"`
	Revision int64  `json:"revision"`

	// RevisionHash record the hash value of the spec of ApplicationRevision object.
	RevisionHash string `json:"revisionHash,omitempty"`
}

// RawComponent record raw component
type RawComponent struct {
	// +kubebuilder:validation:EmbeddedResource
	// +kubebuilder:pruning:PreserveUnknownFields
	Raw runtime.RawExtension `json:"raw"`
}

// WorkflowStepStatus record the status of a workflow step
type WorkflowStepStatus struct {
	ID    string            `json:"id"`
	Name  string            `json:"name,omitempty"`
	Type  string            `json:"type,omitempty"`
	Phase WorkflowStepPhase `json:"phase,omitempty"`
	// A human readable message indicating details about why the workflowStep is in this state.
	Message string `json:"message,omitempty"`
	// A brief CamelCase message indicating details about why the workflowStep is in this state.
	Reason   string          `json:"reason,omitempty"`
	SubSteps *SubStepsStatus `json:"subSteps,omitempty"`
	// FirstExecuteTime is the first time this step execution.
	FirstExecuteTime metav1.Time `json:"firstExecuteTime,omitempty"`
	// LastExecuteTime is the last time this step execution.
	LastExecuteTime metav1.Time `json:"lastExecuteTime,omitempty"`
}

// WorkflowSubStepStatus record the status of a workflow step
type WorkflowSubStepStatus struct {
	ID    string            `json:"id"`
	Name  string            `json:"name,omitempty"`
	Type  string            `json:"type,omitempty"`
	Phase WorkflowStepPhase `json:"phase,omitempty"`
	// A human readable message indicating details about why the workflowStep is in this state.
	Message string `json:"message,omitempty"`
	// A brief CamelCase message indicating details about why the workflowStep is in this state.
	Reason string `json:"reason,omitempty"`
}

// AppStatus defines the observed state of Application
type AppStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	condition.ConditionedStatus `json:",inline"`

	// The generation observed by the application controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	Phase ApplicationPhase `json:"status,omitempty"`

	// Components record the related Components created by Application Controller
	Components []corev1.ObjectReference `json:"components,omitempty"`

	// Services record the status of the application services
	Services []ApplicationComponentStatus `json:"services,omitempty"`

	// Workflow record the status of workflow
	Workflow *WorkflowStatus `json:"workflow,omitempty"`

	// LatestRevision of the application configuration it generates
	// +optional
	LatestRevision *Revision `json:"latestRevision,omitempty"`

	// AppliedResources record the resources that the  workflow step apply.
	AppliedResources []ClusterObjectReference `json:"appliedResources,omitempty"`

	// PolicyStatus records the status of policy
	PolicyStatus []PolicyStatus `json:"policy,omitempty"`
}

// PolicyStatus records the status of policy
type PolicyStatus struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Status *runtime.RawExtension `json:"status,omitempty"`
}

// WorkflowStep defines how to execute a workflow step.
type WorkflowStep struct {
	// Name is the unique name of the workflow step.
	Name string `json:"name"`

	Type string `json:"type"`

	// +kubebuilder:pruning:PreserveUnknownFields
	Properties *runtime.RawExtension `json:"properties,omitempty"`

	DependsOn []string `json:"dependsOn,omitempty"`

	Inputs StepInputs `json:"inputs,omitempty"`

	Outputs StepOutputs `json:"outputs,omitempty"`
}

// WorkflowStatus record the status of workflow
type WorkflowStatus struct {
	AppRevision string       `json:"appRevision,omitempty"`
	Mode        WorkflowMode `json:"mode"`
	Message     string       `json:"message,omitempty"`

	Suspend    bool `json:"suspend"`
	Terminated bool `json:"terminated"`
	Finished   bool `json:"finished"`

	ContextBackend *corev1.ObjectReference `json:"contextBackend,omitempty"`
	Steps          []WorkflowStepStatus    `json:"steps,omitempty"`

	StartTime metav1.Time `json:"startTime,omitempty"`
}

// SubStepsStatus record the status of workflow steps.
type SubStepsStatus struct {
	StepIndex int                     `json:"stepIndex,omitempty"`
	Mode      WorkflowMode            `json:"mode,omitempty"`
	Steps     []WorkflowSubStepStatus `json:"steps,omitempty"`
}

// WorkflowStepPhase describes the phase of a workflow step.
type WorkflowStepPhase string

const (
	// WorkflowStepPhaseSucceeded will make the controller run the next step.
	WorkflowStepPhaseSucceeded WorkflowStepPhase = "succeeded"
	// WorkflowStepPhaseFailed will report error in `message`.
	WorkflowStepPhaseFailed WorkflowStepPhase = "failed"
	// WorkflowStepPhaseStopped will make the controller stop the workflow.
	WorkflowStepPhaseStopped WorkflowStepPhase = "stopped"
	// WorkflowStepPhaseRunning will make the controller continue the workflow.
	WorkflowStepPhaseRunning WorkflowStepPhase = "running"
)

// DefinitionType describes the type of DefinitionRevision.
// +kubebuilder:validation:Enum=Component;Trait;Policy;WorkflowStep
type DefinitionType string

const (
	// ComponentType represents DefinitionRevision refer to type ComponentDefinition
	ComponentType DefinitionType = "Component"

	// TraitType represents DefinitionRevision refer to type TraitDefinition
	TraitType DefinitionType = "Trait"

	// PolicyType represents DefinitionRevision refer to type PolicyDefinition
	PolicyType DefinitionType = "Policy"

	// WorkflowStepType represents DefinitionRevision refer to type WorkflowStepDefinition
	WorkflowStepType DefinitionType = "WorkflowStep"
)

// WorkflowMode describes the mode of workflow
type WorkflowMode string

const (
	// WorkflowModeDAG describes the DAG mode of workflow
	WorkflowModeDAG WorkflowMode = "DAG"
	// WorkflowModeStep describes the step by step mode of workflow
	WorkflowModeStep WorkflowMode = "StepByStep"
)

// AppRolloutStatus defines the observed state of AppRollout
type AppRolloutStatus struct {
	v1alpha1.RolloutStatus `json:",inline"`

	// LastUpgradedTargetAppRevision contains the name of the app that we upgraded to
	// We will restart the rollout if this is not the same as the spec
	LastUpgradedTargetAppRevision string `json:"lastTargetAppRevision"`

	// LastSourceAppRevision contains the name of the app that we need to upgrade from.
	// We will restart the rollout if this is not the same as the spec
	LastSourceAppRevision string `json:"LastSourceAppRevision,omitempty"`
}

// ApplicationTrait defines the trait of application
type ApplicationTrait struct {
	Type string `json:"type"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Properties *runtime.RawExtension `json:"properties,omitempty"`
}

// ApplicationComponent describe the component of application
type ApplicationComponent struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// ExternalRevision specified the component revisionName
	ExternalRevision string `json:"externalRevision,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Properties *runtime.RawExtension `json:"properties,omitempty"`

	DependsOn []string    `json:"dependsOn,omitempty"`
	Inputs    StepInputs  `json:"inputs,omitempty"`
	Outputs   StepOutputs `json:"outputs,omitempty"`

	// Traits define the trait of one component, the type must be array to keep the order.
	Traits []ApplicationTrait `json:"traits,omitempty"`

	// +kubebuilder:pruning:PreserveUnknownFields
	// scopes in ApplicationComponent defines the component-level scopes
	// the format is <scope-type:scope-instance-name> pairs, the key represents type of `ScopeDefinition` while the value represent the name of scope instance.
	Scopes map[string]string `json:"scopes,omitempty"`
}

// StepOutputs defines output variable of WorkflowStep
type StepOutputs []outputItem

// StepInputs defines variable input of WorkflowStep
type StepInputs []inputItem

type inputItem struct {
	ParameterKey string `json:"parameterKey"`
	From         string `json:"from"`
}

type outputItem struct {
	ValueFrom string `json:"valueFrom"`
	Name      string `json:"name"`
}

// ClusterSelector defines the rules to select a Cluster resource.
// Either name or labels is needed.
type ClusterSelector struct {
	// Name is the name of the cluster.
	Name string `json:"name,omitempty"`

	// Labels defines the label selector to select the cluster.
	Labels map[string]string `json:"labels,omitempty"`
}

// Distribution defines the replica distribution of an AppRevision to a cluster.
type Distribution struct {
	// Replicas is the replica number.
	Replicas int `json:"replicas,omitempty"`
}

// ClusterPlacement defines the cluster placement rules for an app revision.
type ClusterPlacement struct {
	// ClusterSelector selects the cluster to  deploy apps to.
	// If not specified, it indicates the host cluster per se.
	ClusterSelector *ClusterSelector `json:"clusterSelector,omitempty"`

	// Distribution defines the replica distribution of an AppRevision to a cluster.
	Distribution Distribution `json:"distribution,omitempty"`
}

// ResourceCreatorRole defines the resource creator.
type ResourceCreatorRole string

const (
	// PolicyResourceCreator create the policy resource.
	PolicyResourceCreator ResourceCreatorRole = "policy"
	// WorkflowResourceCreator create the resource in workflow.
	WorkflowResourceCreator ResourceCreatorRole = "workflow"
	// DebugResourceCreator create the debug resource.
	DebugResourceCreator ResourceCreatorRole = "debug"
)

// OAMObjectReference defines the object reference for an oam resource
type OAMObjectReference struct {
	Component string `json:"component,omitempty"`
	Trait     string `json:"trait,omitempty"`
	Env       string `json:"env,omitempty"`
}

// Equal check if two references are equal
func (in OAMObjectReference) Equal(r OAMObjectReference) bool {
	return in.Component == r.Component && in.Trait == r.Trait && in.Env == r.Env
}

// AddLabelsToObject add labels to object if properties are not empty
func (in OAMObjectReference) AddLabelsToObject(obj client.Object) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	if in.Component != "" {
		labels[oam.LabelAppComponent] = in.Component
	}
	if in.Trait != "" {
		labels[oam.TraitTypeLabel] = in.Trait
	}
	if in.Env != "" {
		labels[oam.LabelAppEnv] = in.Env
	}
	obj.SetLabels(labels)
}

// NewOAMObjectReferenceFromObject create OAMObjectReference from object
func NewOAMObjectReferenceFromObject(obj client.Object) OAMObjectReference {
	if labels := obj.GetLabels(); labels != nil {
		return OAMObjectReference{
			Component: labels[oam.LabelAppComponent],
			Trait:     labels[oam.TraitTypeLabel],
			Env:       labels[oam.LabelAppEnv],
		}
	}
	return OAMObjectReference{}
}

// ClusterObjectReference defines the object reference with cluster.
type ClusterObjectReference struct {
	Cluster                string              `json:"cluster,omitempty"`
	Creator                ResourceCreatorRole `json:"creator,omitempty"`
	corev1.ObjectReference `json:",inline"`
}

// Equal check if two references are equal
func (in ClusterObjectReference) Equal(r ClusterObjectReference) bool {
	return in.APIVersion == r.APIVersion && in.Kind == r.Kind && in.Name == r.Name && in.Namespace == r.Namespace && in.UID == r.UID && in.Creator == r.Creator && in.Cluster == r.Cluster
}

// RawExtensionPointer is the pointer of raw extension
type RawExtensionPointer struct {
	RawExtension *runtime.RawExtension
}

// MarshalJSON may get called on pointers or values, so implement MarshalJSON on value.
// http://stackoverflow.com/questions/21390979/custom-marshaljson-never-gets-called-in-go
func (re RawExtensionPointer) MarshalJSON() ([]byte, error) {
	if re.RawExtension == nil {
		return nil, nil
	}
	if re.RawExtension.Raw == nil {
		// TODO: this is to support legacy behavior of JSONPrinter and YAMLPrinter, which
		// expect to call json.Marshal on arbitrary versioned objects (even those not in
		// the scheme). pkg/kubectl/resource#AsVersionedObjects and its interaction with
		// kubectl get on objects not in the scheme needs to be updated to ensure that the
		// objects that are not part of the scheme are correctly put into the right form.
		if re.RawExtension.Object != nil {
			return json.Marshal(re.RawExtension.Object)
		}
		return []byte("null"), nil
	}
	// TODO: Check whether ContentType is actually JSON before returning it.
	return re.RawExtension.Raw, nil
}

// ApplicationConditionType is a valid value for ApplicationCondition.Type
type ApplicationConditionType int

const (
	// ParsedCondition indicates whether the parsing  is successful.
	ParsedCondition ApplicationConditionType = iota
	// RevisionCondition indicates whether the generated revision is successful.
	RevisionCondition
	// PolicyCondition indicates whether policy processing is successful.
	PolicyCondition
	// RenderCondition indicates whether render processing is successful.
	RenderCondition
	// WorkflowCondition indicates whether workflow processing is successful.
	WorkflowCondition
	// RolloutCondition indicates whether rollout processing is successful.
	RolloutCondition
	// ReadyCondition indicates whether whole application processing is successful.
	ReadyCondition
)

var conditions = map[ApplicationConditionType]string{
	ParsedCondition:   "Parsed",
	RevisionCondition: "Revision",
	PolicyCondition:   "Policy",
	RenderCondition:   "Render",
	WorkflowCondition: "Workflow",
	RolloutCondition:  "Rollout",
	ReadyCondition:    "Ready",
}

// String returns the string corresponding to the condition type.
func (ct ApplicationConditionType) String() string {
	return conditions[ct]
}

// ParseApplicationConditionType parse ApplicationCondition Type.
func ParseApplicationConditionType(s string) (ApplicationConditionType, error) {
	for k, v := range conditions {
		if v == s {
			return k, nil
		}
	}
	return -1, errors.New("unknown condition type")
}

// ReferredObject the referred Kubernetes object
type ReferredObject struct {
	// +kubebuilder:validation:EmbeddedResource
	// +kubebuilder:pruning:PreserveUnknownFields
	runtime.RawExtension `json:",inline"`
}

// ReferredObjectList a list of referred Kubernetes objects
type ReferredObjectList struct {
	// Objects a list of Kubernetes objects.
	// +optional
	Objects []ReferredObject `json:"objects,omitempty"`
}
