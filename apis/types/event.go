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

package types

// reason for Application
const (
	ReasonParsed      = "Parsed"
	ReasonRendered    = "Rendered"
	ReasonRevisoned   = "Revisioned"
	ReasonApplied     = "Applied"
	ReasonHealthCheck = "HealthChecked"
	ReasonDeployed    = "Deployed"
	ReasonRollout     = "Rollout"

	ReasonFailedParse           = "FailedParse"
	ReasonFailedRender          = "FailedRender"
	ReasonFailedRevision        = "FailedRevision"
	ReasonFailedWorkflow        = "FailedWorkflow"
	ReasonFailedApply           = "FailedApply"
	ReasonFailedHealthCheck     = "FailedHealthCheck"
	ReasonFailedGC              = "FailedGC"
	ReasonFailedToHandleRollout = "FailedRollout"
)

// reason for ApplicationRollout
const (
	ReasonReconcileRollout             = "AppRolloutReconciled"
	ReasonFindAppRollout               = "appRolloutFound"
	ReasonGenerateManifest             = "ManifestGenerated"
	ReasonDetermineRolloutComponent    = "RolloutComponentDetermined"
	ReasonAssembleManifests            = "ManifestsAssembled"
	ReasonTemplateSourceMenifest       = "SourceManifestTemplated"
	ReasonTemplateTargetMenifest       = "TargetManifestTemplated"
	ReasonFetchSourceAndTargetWorkload = "SourceAndTargetWorkloadFetched"
	ReasonSuccessRollout               = "RolloutSucceeded"

	ReasonFailedReconcileRollout             = "FailedAppRolloutReconcile"
	ReasonNotFoundAppRollout                 = "AppRollOutNotExist"
	ReasonFailedGenerateManifest             = "FailedManifestGenerate"
	ReasonFailedDetermineRolloutComponent    = "FailedRolloutComponentDetermine"
	ReasonFailedAssembleManifests            = "FailedManifestsAssemble"
	ReasonFailedTemplateSourceMenifest       = "FailedSourceMenifestTemplate"
	ReasonFailedTemplateTargetMenifest       = "FailedTargetMenifestTemplate"
	ReasonFailedFetchSourceAndTargetWorkload = "FailedSourceAndTargetWorkloadFetch"
	ReasonFailedRollout                      = "FailedRollout"
)

// event message for Application
const (
	MessageParsed      = "Parsed successfully"
	MessageRendered    = "Rendered successfully"
	MessageRevisioned  = "Revisioned successfully"
	MessageApplied     = "Applied successfully"
	MessageHealthCheck = "Health checked healthy"
	MessageDeployed    = "Deployed successfully"
	MessageRollout     = "Rollout successfully"

	MessageFailedParse       = "fail to parse application, err: %v"
	MessageFailedRender      = "fail to render application, err: %v"
	MessageFailedRevision    = "fail to handle application revision, err: %v"
	MessageFailedApply       = "fail to apply component, err: %v"
	MessageFailedHealthCheck = "fail to health check, err: %v"
	MessageFailedGC          = "fail to garbage collection, err: %v"
)

// event message for ApplicationRollout
const (
	MessasgeReconciledRollout             = "AppRollOut reconciled successfully"
	MessageGeneratedManifest              = "Manifest generated successfully"
	MessageDeterminedRolloutComponent     = "Rollout component determined successfully"
	MessageAssembleManifests              = "Manifests assembled successfully"
	MessageTemplatedSourceManifest        = "Source manifest templated successfully"
	MessageTemplatedTargetManifest        = "Target manifest templated successfully"
	MessageFetchedSourceAndTargetWorkload = "Source and target workload fetched successfully"
	MessageSucceededRollout               = "Rollout succeeded"

	MessageFailedReconcileRollout             = "fail to reconcile AppRollOut, err: %v"
	MessageFailedGenerateManifest             = "fail to generate source or target manifest, err: %v"
	MessageFailedDetermineRolloutComponent    = "fail to determine Rollout component, err: %v"
	MessageFailedAssembleManifests            = "fail to assemble manifests"
	MessageFailedTemplateSourceManifest       = "fail to template source manifest, err: %v"
	MessageFailedTemplateTargetManifest       = "fail to template target manifest, err: %v"
	MessageFailedFetchSourceAndTargetWorkload = "fail to fetch source and target workload, err: %v"
	MessageFailedRollout                      = "fail to rollout, err: %v"
)
