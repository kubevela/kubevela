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

import "k8s.io/klog/v2"

const (
	// LogInfo level is for most info logs, this is the default
	// One should just call Info directly.
	LogInfo klog.Level = iota

	// LogDebug is for more verbose logs
	LogDebug

	// LogDebugWithContent is recommended if one wants to log with the content of the object,
	// ie. http body, json/yaml file content
	LogDebugWithContent

	// LogTrace is the most verbose log level, don't add anything after this
	LogTrace = 100
)
