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

// HealthStatusCode Represents resource health status
type HealthStatusCode string

const (
	// HealthStatusHealthy  resource is healthy
	HealthStatusHealthy HealthStatusCode = "HealthStatusHealthy"
	// HealthStatusUnHealthy resource is unhealthy
	HealthStatusUnHealthy HealthStatusCode = "HealthStatusUnHealthy"
	// HealthStatusProgressing resource is still progressing
	HealthStatusProgressing HealthStatusCode = "HealthStatusProgressing"
	// HealthStatusUnKnown health status is unknown
	HealthStatusUnKnown HealthStatusCode = "HealthStatusUnKnown"
)

// HealthStatus the resource health status
type HealthStatus struct {
	Status  HealthStatusCode `json:"statusCode"`
	Reason  string           `json:"reason"`
	Message string           `json:"message"`
}
