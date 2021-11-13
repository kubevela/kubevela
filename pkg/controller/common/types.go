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
	// RollingComponentsSep is the separator that divide the names in the newComponent annotation
	RollingComponentsSep = ","
	// DisableAllCaps disable all capabilities
	DisableAllCaps = "all"
	// DisableNoneCaps disable none of capabilities
	DisableNoneCaps = ""
	// HealthScopeControllerName is the controller name of healthScope controller
	HealthScopeControllerName = "healthscope"
	// RolloutControllerName is the controller name of rollout controller
	RolloutControllerName = "rollout"
	// EnvBindingControllerName is the controller name of envbinding
	EnvBindingControllerName = "envbinding"
)
