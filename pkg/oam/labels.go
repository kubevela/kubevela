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
	// LabelAppComponent records the name of Component
	LabelAppComponent = "app.oam.dev/component"
	// LabelAppComponentRevision records the revision name of Component
	LabelAppComponentRevision = "app.oam.dev/revision"
	// LabelOAMResourceType whether a CR is workload or trait
	LabelOAMResourceType = "app.oam.dev/resourceType"
	// LabelAppConfigHash records the Hash value of the application configuration
	LabelAppConfigHash = "app.oam.dev/appConfig-hash"

	// WorkloadTypeLabel indicates the type of the workloadDefinition
	WorkloadTypeLabel = "workload.oam.dev/type"
	// TraitTypeLabel indicates the type of the traitDefinition
	TraitTypeLabel = "trait.oam.dev/type"
	// TraitResource indicates which resource it is when a trait is composed by multiple resources in KubeVela
	TraitResource = "trait.oam.dev/resource"
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
	// the application controller should not reconcile it yet
	AnnotationAppRollout = "app.oam.dev/rollout-template"

	// AnnotationNewAppConfig indicates that the application configuration is new
	// this is to enable the applicationConfiguration controller to handle the
	// first reconcile logic differently similar to what "finalize" field
	AnnotationNewAppConfig = "app.oam.dev/new-appConfig"

	// AnnotationNewComponent indicates that the component is new
	// this is to enable any concerned controllers to handle the first component apply logic differently
	// the value of the annotation is name of the component revision
	AnnotationNewComponent = "app.oam.dev/new-component"
)
