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

import (
	"time"
)

var (
	// PerfEnabled identify whether to add performance log for controllers
	PerfEnabled = false
)

var (
	// ApplicationReSyncPeriod re-sync period to reconcile application
	ApplicationReSyncPeriod = time.Minute * 5
	// ApplicationDefaultAutoRevision is whether policy-rendered spec changes cut a new
	// ApplicationRevision for apps without the policy.oam.dev/auto-revision annotation.
	ApplicationDefaultAutoRevision = false
	// ApplicationDefaultApplyPolicyUpdates is whether apps without the
	// policy.oam.dev/apply-policy-updates annotation adopt new policy versions.
	ApplicationDefaultApplyPolicyUpdates = false
	// ApplicationDefaultApplyDataChanges is whether apps without the
	// policy.oam.dev/apply-data-changes annotation take changed policy output.
	ApplicationDefaultApplyDataChanges = false
	// AllowNamespaceGlobalPolicies is whether global Application-scoped PolicyDefinitions outside
	// vela-system apply (to the Applications in their namespace). Off, global policies are
	// platform-wide only: they live in vela-system, and the webhook refuses new ones elsewhere.
	// On by default in 1.11 so a patch upgrade keeps namespace globals working.
	AllowNamespaceGlobalPolicies = true
	// AllowSkipGlobalPolicies is whether an Application may opt out of global policies with the
	// policy.oam.dev/skip-global annotation. Off, global policies are mandatory: the webhook
	// refuses adding the annotation and the controller ignores it. On by default in 1.11 so a patch
	// upgrade keeps the opt-out working.
	AllowSkipGlobalPolicies = true
)
