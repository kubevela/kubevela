/*
Copyright 2019 The Crossplane Authors.

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

package oam

// Label key strings.
// AppConfig controller will add these labels into workloads.
const (
	// LabelAppName records the name of AppConfig
	LabelAppName = "app.oam.dev/name"
	// LabelAppRevision records the name of Application, it's equal to name of AppConfig created by Application
	LabelAppRevision = "app.oam.dev/appRevision"
	// LabelAppDeployment records the name of AppDeployment.
	LabelAppDeployment = "app.oam.dev/appDeployment"
	// LabelAppComponent records the name of Component
	LabelAppComponent = "app.oam.dev/component"
	// LabelAppComponentRevision records the revision name of Component
	LabelAppComponentRevision = "app.oam.dev/revision"
	// LabelOAMResourceType whether a CR is workload or trait
	LabelOAMResourceType = "app.oam.dev/resourceType"
	// LabelAppRevisionHash records the Hash value of the application revision
	LabelAppRevisionHash = "app.oam.dev/app-revision-hash"
	// LabelAppNamespace records the namespace of Application
	LabelAppNamespace = "app.oam.dev/namesapce"

	// WorkloadTypeLabel indicates the type of the workloadDefinition
	WorkloadTypeLabel = "workload.oam.dev/type"
	// TraitTypeLabel indicates the type of the traitDefinition
	TraitTypeLabel = "trait.oam.dev/type"
	// TraitResource indicates which resource it is when a trait is composed by multiple resources in KubeVela
	TraitResource = "trait.oam.dev/resource"

	// LabelComponentDefinitionName records the name of ComponentDefinition
	LabelComponentDefinitionName = "componentdefinition.oam.dev/name"
	// LabelTraitDefinitionName records the name of TraitDefinition
	LabelTraitDefinitionName = "trait.oam.dev/name"
	// LabelPolicyDefinitionName records the name of PolicyDefinition
	LabelPolicyDefinitionName = "policydefinition.oam.dev/name"
	// LabelWorkflowStepDefinitionName records the name of WorkflowStepDefinition
	LabelWorkflowStepDefinitionName = "workflowstepdefinition.oam.dev/name"

	// LabelControllerRevisionComponent indicate which component the revision belong to
	LabelControllerRevisionComponent = "controller.oam.dev/component"
	// LabelComponentRevisionHash records the hash value of a component
	LabelComponentRevisionHash = "app.oam.dev/component-revision-hash"

	// LabelAddonsName records the name of initializer stored in configMap
	LabelAddonsName = "addons.oam.dev/type"
)

const (
	// ResourceTypeTrait mark this K8s Custom Resource is an OAM trait
	ResourceTypeTrait = "TRAIT"
	// ResourceTypeWorkload mark this K8s Custom Resource is an OAM workload
	ResourceTypeWorkload = "WORKLOAD"
)

const (
	// AnnotationAppGeneration records the generation of AppConfig
	AnnotationAppGeneration = "app.oam.dev/generation"

	// AnnotationLastAppliedConfig records the previous configuration of a
	// resource for use in a three way diff during a patching apply
	AnnotationLastAppliedConfig = "app.oam.dev/last-applied-configuration"

	// AnnotationAppRollout indicates that the application is still rolling out
	// the application controller should treat it differently
	AnnotationAppRollout = "app.oam.dev/rollout-template"

	// AnnotationInplaceUpgrade indicates the workload should upgrade with the the same name
	// the name of the workload instance should not changing along with the revision
	AnnotationInplaceUpgrade = "app.oam.dev/inplace-upgrade"

	// AnnotationRollingComponent indicates that the component is rolling out
	// this is to enable any concerned controllers to handle the first component apply logic differently
	// the value of the annotation is a list of component name of all the new component
	AnnotationRollingComponent = "app.oam.dev/rolling-components"

	// AnnotationAppRevision indicates that the object is an application revision
	//	its controller should not try to reconcile it
	AnnotationAppRevision = "app.oam.dev/app-revision"

	// AnnotationAppRevisionOnly the Application update should only generate revision,
	// not any appContexts or components.
	AnnotationAppRevisionOnly = "app.oam.dev/revision-only"

	// AnnotationWorkflowContext is used to pass in the workflow context marshalled in json format.
	AnnotationWorkflowContext = "app.oam.dev/workflow-context"

	// AnnotationKubeVelaVersion is used to record current KubeVela version
	AnnotationKubeVelaVersion = "oam.dev/kubevela-version"

	// AnnotationFilterAnnotationKeys is used to filter annotations passed to workload and trait, split by comma
	AnnotationFilterAnnotationKeys = "filter.oam.dev/annotation-keys"

	// AnnotationFilterLabelKeys is used to filter labels passed to workload and trait, split by comma
	AnnotationFilterLabelKeys = "filter.oam.dev/label-keys"
)
