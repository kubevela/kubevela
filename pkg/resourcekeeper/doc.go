/*
Copyright 2026 The KubeVela Authors.

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

// Package resourcekeeper applies an owner's resources, records them in ResourceTrackers and
// garbage-collects the ones it no longer renders.
//
// It knows no owner kind. An owner implements resourcetracker.Tracked (pkg/appkeeper for
// Applications, and a Component in the spoke agent), supplies its apply-time Policies, and
// fills the Options hooks for whatever the library cannot do for it: revision collection,
// dependency order, the requester's identity, metrics.
//
// Together with pkg/resourcetracker, pkg/utils/apply and pkg/kubeutil it is meant to move to
// its own module, so it may depend only on those and on the API packages (apis/core.oam.dev,
// pkg/oam, pkg/utils/errors). Three tests in boundary_test.go enforce that: the imports, the
// weight of the API packages, and that no identifier here names the Application.
//
// # Migration: owner labels
//
// Objects written before owner.oam.dev/* labels carry app.oam.dev/* only. Five pieces carry
// that upgrade, each marked "Migration: owner labels" where it sits. All five can go once the
// oldest supported release writes owner labels:
//
//   - resourceKeeper.labelTrackers, which labels an owner's trackers when it loads them;
//   - the Stamp in resourceKeeper.StateKeep, so an idle owner's resources are relabelled;
//   - applicationTracked.ControlledBy's fall back to app.oam.dev/* (pkg/appkeeper);
//   - applicationTracked.Release stripping app.oam.dev/* as well;
//   - apply.Config.LegacyControlledBy, so other kinds see an unlabelled resource as its
//     Application's rather than as unowned.
package resourcekeeper
