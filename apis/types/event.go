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
	ReasonApplied     = "Applied"
	ReasonHealthCheck = "HealthChecked"
	ReasonDeployed    = "Deployed"

	ReasonFailedParse       = "FailedParse"
	ReasonFailedRender      = "FailedRender"
	ReasonFailedApply       = "FailedApply"
	ReasonFailedHealthCheck = "FailedHealthCheck"
	ReasonFailedGC          = "FailedGC"
)

// event message for Application
const (
	MessageParsed      = "Parsed successfully"
	MessageRendered    = "Rendered successfully"
	MessageApplied     = "Applied successfully"
	MessageHealthCheck = "Health checked healthy"
	MessageDeployed    = "Deployed successfully"

	MessageFailedParse       = "fail to parse application, err: %v"
	MessageFailedRender      = "fail to render application, err: %v"
	MessageFailedApply       = "fail to apply component, err: %v"
	MessageFailedHealthCheck = "fail to health check, err: %v"
	MessageFailedGC          = "fail to garbage collection, err: %v"
)
