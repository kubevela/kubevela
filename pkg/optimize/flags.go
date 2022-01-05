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

package optimize

import (
	"flag"
)

// AddFlags add flags
func AddFlags() {
	// optimize client
	flag.BoolVar(&ResourceTrackerOptimizer.OptimizeListOp, "optimize-resource-tracker-list-op", false, "Optimize ResourceTracker List Op by adding index. This will increase the use of memory and accelerate the list operation of ResourceTracker.")

	// optimize functions
	flag.BoolVar(&RevisionOptimizer.DisableAllComponentRevision, "optimize-disable-component-revision", false, "Optimize ComponentRevision by disabling the creation and gc. Side effect: rollout cannot be used.")
	flag.BoolVar(&RevisionOptimizer.DisableAllApplicationRevision, "optimize-disable-application-revision", false, "Optimize Application by disabling the creation and gc. Side effect: application cannot rollback.")
	flag.BoolVar(&RevisionOptimizer.DisableWorkflowRecorder, "optimize-disable-workflow-recorder", false, "Optimize workflow recorder by disabling the creation and gc. Side effect: workflow will not record application after finished running.")
}
