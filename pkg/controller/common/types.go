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

const (
	// AutoscaleControllerName is the controller name of Trait autoscale
	AutoscaleControllerName = "autoscale"
	// MetricsControllerName is the controller name of Trait metrics
	MetricsControllerName = "metrics"
	// PodspecWorkloadControllerName is the controller name of Workload podsepcworkload
	PodspecWorkloadControllerName = "podspecworkload"
	// RouteControllerName is the controller name of Trait route
	RouteControllerName = "route"
	// RollingComponentsSep is the separator that divide the names in the newComponent annotation
	RollingComponentsSep = ","
	// DisableAllCaps disable all capabilities
	DisableAllCaps = "all"
	// DisableNoneCaps disable none of capabilities
	DisableNoneCaps = ""
	// ManualScalerTraitControllerName is the controller name of manualScalerTrait
	ManualScalerTraitControllerName = "manualscaler"
	// ContainerizedWorkloadControllerName is the controller name of containerized workload
	ContainerizedWorkloadControllerName = "containerizedwokrload"
	// HealthScopeControllerName is the controller name of healthScope controller
	HealthScopeControllerName = "healthscope"
	// RolloutControllerName is the controller name of rollout controller
	RolloutControllerName = "rollout"
)
