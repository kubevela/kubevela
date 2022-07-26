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

package features

import (
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/featuregate"
)

const (
	// Compatibility Features

	// DeprecatedPolicySpec enable the use of deprecated policy spec
	DeprecatedPolicySpec featuregate.Feature = "DeprecatedPolicySpec"
	// LegacyObjectTypeIdentifier enable the use of legacy object type identifier for selecting ref-object
	LegacyObjectTypeIdentifier featuregate.Feature = "LegacyObjectTypeIdentifier"
	// DeprecatedObjectLabelSelector enable the use of deprecated object label selector for selecting ref-object
	DeprecatedObjectLabelSelector featuregate.Feature = "DeprecatedObjectLabelSelector"
	// LegacyResourceTrackerGC enable the gc of legacy resource tracker in managed clusters
	LegacyResourceTrackerGC featuregate.Feature = "LegacyResourceTrackerGC"
	// EnableSuspendOnFailure enable suspend on workflow failure
	EnableSuspendOnFailure featuregate.Feature = "EnableSuspendOnFailure"
	// LegacyComponentRevision if enabled, create component revision even no rollout trait attached
	LegacyComponentRevision featuregate.Feature = "LegacyComponentRevision"
	// LegacyResourceOwnerValidation if enabled, the resource dispatch will allow existing resource not to have owner
	// application and the current application will take over it
	LegacyResourceOwnerValidation featuregate.Feature = "LegacyResourceOwnerValidation"
	// DisableReferObjectsFromURL if set, the url ref objects will be disallowed
	DisableReferObjectsFromURL featuregate.Feature = "DisableReferObjectsFromURL"

	// Edge Features

	// AuthenticateApplication enable the authentication for application
	AuthenticateApplication featuregate.Feature = "AuthenticateApplication"
)

var defaultFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	DeprecatedPolicySpec:          {Default: false, PreRelease: featuregate.Alpha},
	LegacyObjectTypeIdentifier:    {Default: false, PreRelease: featuregate.Alpha},
	DeprecatedObjectLabelSelector: {Default: false, PreRelease: featuregate.Alpha},
	LegacyResourceTrackerGC:       {Default: false, PreRelease: featuregate.Beta},
	EnableSuspendOnFailure:        {Default: false, PreRelease: featuregate.Alpha},
	LegacyComponentRevision:       {Default: false, PreRelease: featuregate.Alpha},
	LegacyResourceOwnerValidation: {Default: false, PreRelease: featuregate.Alpha},
	DisableReferObjectsFromURL:    {Default: false, PreRelease: featuregate.Alpha},
	AuthenticateApplication:       {Default: false, PreRelease: featuregate.Alpha},
}

func init() {
	runtime.Must(feature.DefaultMutableFeatureGate.Add(defaultFeatureGates))
}
