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
	ReasonParsed          = "Parsed"
	ReasonRendered        = "Rendered"
	ReasonPolicyGenerated = "PolicyGenerated"
	ReasonRevisoned       = "Revisioned"
	ReasonApplied         = "Applied"
	ReasonHealthCheck     = "HealthChecked"
	ReasonDeployed        = "Deployed"
	ReasonRollout         = "Rollout"

	ReasonFailedParse       = "FailedParse"
	ReasonFailedRender      = "FailedRender"
	ReasonFailedRevision    = "FailedRevision"
	ReasonFailedWorkflow    = "FailedWorkflow"
	ReasonFailedApply       = "FailedApply"
	ReasonFailedHealthCheck = "FailedHealthCheck"
	ReasonFailedStateKeep   = "FailedStateKeep"
	ReasonFailedGC          = "FailedGC"
	ReasonFailedRollout     = "FailedRollout"
)

// event message for Application
const (
	MessageParsed           = "Parsed successfully"
	MessageRendered         = "Rendered successfully"
	MessagePolicyGenerated  = "Policy generated successfully"
	MessageRevisioned       = "Revisioned successfully"
	MessageApplied          = "Applied successfully"
	MessageWorkflowFinished = "Workflow finished"
	MessageHealthCheck      = "Health checked healthy"
	MessageDeployed         = "Deployed successfully"
	MessageRollout          = "Rollout successfully"

	MessageFailedParse       = "fail to parse application, err: %v"
	MessageFailedRender      = "fail to render application, err: %v"
	MessageFailedRevision    = "fail to handle application revision, err: %v"
	MessageFailedApply       = "fail to apply component, err: %v"
	MessageFailedHealthCheck = "fail to health check, err: %v"
	MessageFailedGC          = "fail to garbage collection, err: %v"
)
