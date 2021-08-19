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

	ReasonFailedParse       = "FailedParse"
	ReasonFailedRender      = "FailedRender"
	ReasonFailedRevision    = "FailedRevision"
	ReasonFailedWorkflow    = "FailedWorkflow"
	ReasonFailedApply       = "FailedApply"
	ReasonFailedHealthCheck = "FailedHealthCheck"
	ReasonFailedGC          = "FailedGC"
	ReasonFailedRollout     = "FailedRollout"
)

// reason for ApplicationRollout
const (
	ReasonTemplateTargetMenifest    = "TargetManifestTemplated"
	ReasonStartRolloutPlanReconcile = "RolloutPlanReconcileStarted"

	ReasonFailedTemplateTargetMenifest    = "FailedTargetMenifestTemplate"
	ReasonFailedStartRolloutPlanReconcile = "FailedRolloutPlanReconcileStart"
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
	MessageTemplatedTargetManifest   = "Target manifest templated successfully"
	MessageStartRolloutPlanReconcile = "RolloutPlan reconcile started successfully"

	MessageFailedTemplateTargetManifest    = "fail to template target manifest, err: %v"
	MessageFailedStartRolloutPlanReconcile = "fail to start RolloutPlan reconcile, err: %v"
)
