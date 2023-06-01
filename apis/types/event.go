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
	ReasonDeployed        = "Deployed"

	ReasonFailedParse     = "FailedParse"
	ReasonFailedRevision  = "FailedRevision"
	ReasonFailedWorkflow  = "FailedWorkflow"
	ReasonFailedApply     = "FailedApply"
	ReasonFailedStateKeep = "FailedStateKeep"
	ReasonFailedGC        = "FailedGC"
)

// event message for Application
const (
	MessageParsed           = "Parsed successfully"
	MessageRendered         = "Rendered successfully"
	MessagePolicyGenerated  = "Policy generated successfully"
	MessageRevisioned       = "Revisioned successfully"
	MessageWorkflowFinished = "Workflow finished"
	MessageDeployed         = "Deployed successfully"
)
