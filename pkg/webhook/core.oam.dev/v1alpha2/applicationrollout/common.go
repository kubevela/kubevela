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

package applicationrollout

import (
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
)

// FindCommonComponent finds the common components in both the source and target application
// the source can be nil
func FindCommonComponent(targetApp, sourceApp *v1alpha2.ApplicationConfiguration) []string {
	var commonComponents []string
	if sourceApp == nil {
		for _, comp := range targetApp.Spec.Components {
			commonComponents = append(commonComponents, utils.ExtractComponentName(comp.RevisionName))
		}
		return commonComponents
	}
	// find the common components in both the source and target application
	// write an O(N) algorithm just for fun, totally doesn't worth the extra space
	targetComponents := make(map[string]bool)
	for _, comp := range targetApp.Spec.Components {
		targetComponents[utils.ExtractComponentName(comp.RevisionName)] = true
	}
	for _, comp := range sourceApp.Spec.Components {
		revisionName := utils.ExtractComponentName(comp.RevisionName)
		if targetComponents[revisionName] {
			commonComponents = append(commonComponents, revisionName)
		}
	}
	return commonComponents
}
