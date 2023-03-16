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
	// GzipResourceTracker enables the gzip compression for ResourceTracker. It can be useful if you have large
	// application that needs to dispatch lots of resources or large resources (like CRD or huge ConfigMap),
	// which at the cost of slower processing speed due to the extra overhead for compression and decompression.
	GzipResourceTracker featuregate.Feature = "GzipResourceTracker"
	// ZstdResourceTracker enables the zstd compression for ResourceTracker.
	// Refer to GzipResourceTracker for its use-cases. It is much faster and more
	// efficient than gzip, about 2x faster and compresses to smaller size.
	// If you are dealing with very large ResourceTrackers (1MB or so), it should
	// have almost NO performance penalties compared to no compression at all.
	// If dealing with smaller ResourceTrackers (10KB - 1MB), the performance
	// penalties are minimal.
	ZstdResourceTracker featuregate.Feature = "ZstdResourceTracker"

	// GzipApplicationRevision serves the same purpose as GzipResourceTracker,
	// but for ApplicationRevision.
	GzipApplicationRevision featuregate.Feature = "GzipApplicationRevision"
	// ZstdApplicationRevision serves the same purpose as ZstdResourceTracker,
	// but for ApplicationRevision.
	ZstdApplicationRevision featuregate.Feature = "ZstdApplicationRevision"

	// ApplyOnce enable the apply-once feature for all applications
	// If enabled, no StateKeep will be run, ResourceTracker will also disable the storage of all resource data, only
	// metadata will be kept
	ApplyOnce featuregate.Feature = "ApplyOnce"

	// MultiStageComponentApply enable multi-stage feature for component
	// If enabled, the dispatch of manifests is performed in batches according to the stage
	MultiStageComponentApply featuregate.Feature = "MultiStageComponentApply"

	// PreDispatchDryRun enable dryrun before dispatching resources
	// Enable this flag can help prevent unsuccessful dispatch resources entering resourcetracker and improve the
	// user experiences of gc but at the cost of increasing network requests.
	PreDispatchDryRun featuregate.Feature = "PreDispatchDryRun"

	// ValidateComponentWhenSharding validate component in sharding mode
	// In sharding mode, since ApplicationRevision will not be cached for webhook, the validation of component
	// need to call Kubernetes APIServer which can be slow and take up some network traffic. So by default, the
	// validation of component will be disabled.
	ValidateComponentWhenSharding = "ValidateComponentWhenSharding"

	// DisableWebhookAutoSchedule disable auto schedule for application mutating webhook when sharding enabled
	// If set to true, the webhook will not make auto schedule for applications and users can make customized
	// scheduler for assigning shards to applications
	DisableWebhookAutoSchedule = "DisableWebhookAutoSchedule"

	// DisableBootstrapClusterInfo disable the cluster info bootstrap at the starting of the controller
	DisableBootstrapClusterInfo = "DisableBootstrapClusterInfo"

	// InformerCacheFilterUnnecessaryFields filter unnecessary fields for informer cache
	InformerCacheFilterUnnecessaryFields = "InformerCacheFilterUnnecessaryFields"

	// SharedDefinitionStorageForApplicationRevision use definition cache to reduce duplicated definition storage
	// for application revision, must be used with InformerCacheFilterUnnecessaryFields
	SharedDefinitionStorageForApplicationRevision = "SharedDefinitionStorageForApplicationRevision"

	// DisableWorkflowContextConfigMapCache disable the workflow context's configmap informer cache
	DisableWorkflowContextConfigMapCache = "DisableWorkflowContextConfigMapCache"
)

var defaultFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	DeprecatedPolicySpec:                          {Default: false, PreRelease: featuregate.Alpha},
	LegacyObjectTypeIdentifier:                    {Default: false, PreRelease: featuregate.Alpha},
	DeprecatedObjectLabelSelector:                 {Default: false, PreRelease: featuregate.Alpha},
	LegacyResourceTrackerGC:                       {Default: false, PreRelease: featuregate.Beta},
	LegacyComponentRevision:                       {Default: false, PreRelease: featuregate.Alpha},
	LegacyResourceOwnerValidation:                 {Default: false, PreRelease: featuregate.Alpha},
	DisableReferObjectsFromURL:                    {Default: false, PreRelease: featuregate.Alpha},
	ApplyResourceByUpdate:                         {Default: false, PreRelease: featuregate.Alpha},
	AuthenticateApplication:                       {Default: false, PreRelease: featuregate.Alpha},
	GzipResourceTracker:                           {Default: false, PreRelease: featuregate.Alpha},
	ZstdResourceTracker:                           {Default: false, PreRelease: featuregate.Alpha},
	ApplyOnce:                                     {Default: false, PreRelease: featuregate.Alpha},
	MultiStageComponentApply:                      {Default: false, PreRelease: featuregate.Alpha},
	GzipApplicationRevision:                       {Default: false, PreRelease: featuregate.Alpha},
	ZstdApplicationRevision:                       {Default: false, PreRelease: featuregate.Alpha},
	PreDispatchDryRun:                             {Default: true, PreRelease: featuregate.Alpha},
	ValidateComponentWhenSharding:                 {Default: false, PreRelease: featuregate.Alpha},
	DisableWebhookAutoSchedule:                    {Default: false, PreRelease: featuregate.Alpha},
	DisableBootstrapClusterInfo:                   {Default: false, PreRelease: featuregate.Alpha},
	InformerCacheFilterUnnecessaryFields:          {Default: true, PreRelease: featuregate.Alpha},
	SharedDefinitionStorageForApplicationRevision: {Default: true, PreRelease: featuregate.Alpha},
	DisableWorkflowContextConfigMapCache:          {Default: true, PreRelease: featuregate.Alpha},
}

func init() {
	runtime.Must(feature.DefaultMutableFeatureGate.Add(defaultFeatureGates))
}
