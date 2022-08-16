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

	// ApplyResourceByUpdate enforces the modification of resource through update requests.
	// If not set, the resource modification will use patch requests (three-way-strategy-merge-patch).
	// The side effect of enabling this feature is that the request traffic will increase due to
	// the increase of bytes transferred and the more frequent resource mutation failure due to the
	// potential conflicts.
	// If set, KubeVela controller will enforce strong restriction on the managed resource that external
	// system would be unable to make modifications to the KubeVela managed resource. In other words,
	// no merge for modifications from multiple sources. Only KubeVela keeps the Source-of-Truth for the
	// resource.
	ApplyResourceByUpdate featuregate.Feature = "ApplyResourceByUpdate"

	// Edge Features

	// AuthenticateApplication enable the authentication for application
	AuthenticateApplication featuregate.Feature = "AuthenticateApplication"
	// GzipResourceTracker enable the gzip compression for ResourceTracker. It can be useful if you have large
	// application that needs to dispatch lots of resources or large resources (like CRD or huge ConfigMap),
	// which at the cost of slower processing speed due to the extra overhead for compression and decompression.
	GzipResourceTracker featuregate.Feature = "GzipResourceTracker"
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
	ApplyResourceByUpdate:         {Default: false, PreRelease: featuregate.Alpha},
	AuthenticateApplication:       {Default: false, PreRelease: featuregate.Alpha},
	GzipResourceTracker:           {Default: false, PreRelease: featuregate.Alpha},
}

func init() {
	runtime.Must(feature.DefaultMutableFeatureGate.Add(defaultFeatureGates))
}
