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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
)

// FindCommonComponentWithManifest finds the common components in both the source and target workloads
// the source can be nil
func FindCommonComponentWithManifest(target, source map[string]*unstructured.Unstructured) []string {
	var commonComponents []string
	if source == nil {
		for compName := range target {
			commonComponents = append(commonComponents, compName)
		}
		return commonComponents
	}
	// find the common components in both the source and target components
	// write an O(N) algorithm just for fun, totally doesn't worth the extra space
	targetComponents := make(map[string]bool)
	for comp := range target {
		targetComponents[comp] = true
	}
	for comp := range source {
		if targetComponents[comp] {
			commonComponents = append(commonComponents, comp)
		}
	}
	return commonComponents
}

// FindCommonComponent finds the common components in both the source and target application
// only used for rollout webhook, will delete after refactor rollout webhook
func FindCommonComponent(targetApp, sourceApp []*types.ComponentManifest) []string {
	var commonComponents []string
	if sourceApp == nil {
		for _, comp := range targetApp {
			commonComponents = append(commonComponents, utils.ExtractComponentName(comp.RevisionName))
		}
		return commonComponents
	}
	// find the common components in both the source and target application
	// write an O(N) algorithm just for fun, totally doesn't worth the extra space
	targetComponents := make(map[string]bool)
	for _, comp := range targetApp {
		targetComponents[utils.ExtractComponentName(comp.RevisionName)] = true
	}
	for _, comp := range sourceApp {
		revisionName := utils.ExtractComponentName(comp.RevisionName)
		if targetComponents[revisionName] {
			commonComponents = append(commonComponents, revisionName)
		}
	}
	return commonComponents
}
