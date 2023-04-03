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
	// LabelReplicaKey records the replica key of Component
	LabelReplicaKey = "app.oam.dev/replicaKey"
	// LabelAppComponentRevision records the revision name of Component
	LabelAppComponentRevision = "app.oam.dev/revision"
	// LabelOAMResourceType whether a CR is workload or trait
	LabelOAMResourceType = "app.oam.dev/resourceType"
	// LabelAppRevisionHash records the Hash value of the application revision
	LabelAppRevisionHash = "app.oam.dev/app-revision-hash"
	// LabelAppNamespace records the namespace of Application
	LabelAppNamespace = "app.oam.dev/namespace"
	// LabelAppCluster records the cluster of Application
	LabelAppCluster = "app.oam.dev/cluster"
	// LabelAppUID records the uid of Application
	LabelAppUID = "app.oam.dev/uid"

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
	// LabelManageWorkloadTrait indicates if the trait will manage the lifecycle of the workload
	LabelManageWorkloadTrait = "trait.oam.dev/manage-workload"
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

	// LabelAddonName indicates the name of the corresponding Addon
	LabelAddonName = "addons.oam.dev/name"

	// LabelAddonAuxiliaryName indicates the name of the auxiliary resource in addon app template
	LabelAddonAuxiliaryName = "addons.oam.dev/auxiliary-name"

	// LabelAddonVersion indicates the version of the corresponding  installed Addon
	LabelAddonVersion = "addons.oam.dev/version"

	// LabelAddonRegistry indicates the name of addon-registry
	LabelAddonRegistry = "addons.oam.dev/registry"

	// LabelAppEnv records the name of Env
	LabelAppEnv = "envbinding.oam.dev/env"

	// LabelNamespaceOfEnvName records the env name of namespace
	LabelNamespaceOfEnvName = "namespace.oam.dev/env"

	// LabelNamespaceOfTargetName records the target name of namespace
	LabelNamespaceOfTargetName = "namespace.oam.dev/target"

	// LabelControlPlaneNamespaceUsage mark the usage of the namespace in control plane cluster.
	LabelControlPlaneNamespaceUsage = "usage.oam.dev/control-plane"

	// LabelRuntimeNamespaceUsage mark the usage of the namespace in runtime cluster.
	// A control plane cluster can also be used as runtime cluster
	LabelRuntimeNamespaceUsage = "usage.oam.dev/runtime"

	// LabelConfigType means the config type
	LabelConfigType = "config.oam.dev/type"

	// LabelProject recorde the project the resource belong to
	LabelProject = "core.oam.dev/project"

	// LabelResourceRules defines the configmap is representing the resource topology rules
	LabelResourceRules = "rules.oam.dev/resources"

	// LabelResourceRuleFormat defines the resource format of the resource topology rules
	LabelResourceRuleFormat = "rules.oam.dev/resource-format"

	// LabelControllerName indicates the controller name
	LabelControllerName = "controller.oam.dev/name"

	// LabelPreCheck indicates if the target resource is for pre-check test
	LabelPreCheck = "core.oam.dev/pre-check"
)

const (
	// VelaNamespaceUsageEnv mark the usage of the namespace is used by env.
	VelaNamespaceUsageEnv = "env"
	// VelaNamespaceUsageTarget mark the usage of the namespace is used as delivery target.
	VelaNamespaceUsageTarget = "target"
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
	// resource for use in a three-way diff during a patching apply
	AnnotationLastAppliedConfig = "app.oam.dev/last-applied-configuration"

	// AnnotationLastAppliedTime indicates the last applied time
	AnnotationLastAppliedTime = "app.oam.dev/last-applied-time"

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

	// AnnotationSkipGC is used to tell application to skip gc workload/trait
	AnnotationSkipGC = "app.oam.dev/skipGC"

	// AnnotationDefinitionRevisionName is used to specify the name of DefinitionRevision in component/trait definition
	AnnotationDefinitionRevisionName = "definitionrevision.oam.dev/name"

	// AnnotationAddonsName records the name of initializer stored in configMap
	AnnotationAddonsName = "addons.oam.dev/name"

	// AnnotationLastAppliedConfiguration is kubectl annotations for 3-way merge
	AnnotationLastAppliedConfiguration = "kubectl.kubernetes.io/last-applied-configuration"

	// AnnotationDeployVersion know the version number of the deployment.
	AnnotationDeployVersion = "app.oam.dev/deployVersion"

	// AnnotationPublishVersion is annotation that record the application workflow version.
	AnnotationPublishVersion = "app.oam.dev/publishVersion"

	// AnnotationAutoUpdate is annotation that let application auto update when it finds definition changes
	AnnotationAutoUpdate = "app.oam.dev/autoUpdate"

	// AnnotationWorkflowName specifies the workflow name for execution.
	AnnotationWorkflowName = "app.oam.dev/workflowName"

	// AnnotationAppName specifies the name for application in db.
	// Note: the annotation is only created by velaUX, please don't use it in other Source of Truth.
	AnnotationAppName = "app.oam.dev/appName"

	// AnnotationAppAlias specifies the alias for application in db.
	AnnotationAppAlias = "app.oam.dev/appAlias"

	// AnnotationWorkloadGVK indicates the managed workload's GVK by trait
	AnnotationWorkloadGVK = "trait.oam.dev/workload-gvk"

	// AnnotationWorkloadName indicates the managed workload's name by trait
	AnnotationWorkloadName = "trait.oam.dev/workload-name"

	// AnnotationControllerRequirement indicates the controller version that can process the application/definition.
	AnnotationControllerRequirement = "app.oam.dev/controller-version-require"

	// AnnotationApplicationServiceAccountName indicates the name of the ServiceAccount to use to apply Components and run Workflow.
	// ServiceAccount will be used in the local cluster only.
	AnnotationApplicationServiceAccountName = "app.oam.dev/service-account-name"

	// AnnotationApplicationUsername indicates the username of the Application to use to apply resources
	AnnotationApplicationUsername = "app.oam.dev/username"

	// AnnotationApplicationGroup indicates the group of the Application to use to apply resources
	AnnotationApplicationGroup = "app.oam.dev/group"

	// AnnotationAppSharedBy records who share the application
	AnnotationAppSharedBy = "app.oam.dev/shared-by"

	// AnnotationResourceURL records the source url of the Kubernetes object
	AnnotationResourceURL = "app.oam.dev/resource-url"

	// AnnotationIgnoreWithoutCompKey indicates the bond component.
	// Deprecated: please use AnnotationAddonDefinitionBindCompKey.
	AnnotationIgnoreWithoutCompKey = "addon.oam.dev/ignore-without-component"

	// AnnotationAddonDefinitionBondCompKey indicates the definition in addon bond component.
	AnnotationAddonDefinitionBondCompKey = "addon.oam.dev/bind-component"

	// AnnotationSkipResume annotation indicates that the resource does not need to be resumed.
	AnnotationSkipResume = "controller.core.oam.dev/skip-resume"
)

const (
	// ResourceTopologyFormatYAML mark the format of resource topology is yaml, by default, it's yaml.
	ResourceTopologyFormatYAML = "yaml"
	// ResourceTopologyFormatJSON mark the format of resource topology is json.
	ResourceTopologyFormatJSON = "json"
)

const (
	// FinalizerResourceTracker is the application finalizer for gc
	FinalizerResourceTracker = "app.oam.dev/resource-tracker-finalizer"
	// FinalizerOrphanResource indicates that the gc process should orphan managed
	// resources instead of deleting them
	FinalizerOrphanResource = "app.oam.dev/orphan-resource"
)
