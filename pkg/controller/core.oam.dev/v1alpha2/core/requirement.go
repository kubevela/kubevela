/*
Copyright 2022 The KubeVela Authors.

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

package core

import (
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// MatchControllerRequirement check the requirement
func MatchControllerRequirement(definition util.ConditionedObject, controllerVersion string, ignoreDefNoCtrlReq bool) bool {
	if definition.GetAnnotations() != nil {
		if requireVersion, ok := definition.GetAnnotations()[oam.AnnotationControllerRequirement]; ok {
			return requireVersion == controllerVersion
		}
	}
	if ignoreDefNoCtrlReq {
		return false
	}
	return true
}
