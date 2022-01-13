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

	// optimize controller reconcile loop
	flag.BoolVar(&ControllerOptimizer.EnableReconcileLoopReduction, "optimize-controller-reconcile-loop-reduction", false, "Optimize ApplicationController reconcile by reducing the number loops to reconcile application.")

	// optimize functions
	flag.Float64Var(&ResourceTrackerOptimizer.MarkWithProbability, "optimize-mark-with-prob", 0.0, "Optimize ResourceTracker GC by only run mark with probability. Default to 0.0 means not enable it. Side effect: outdated ResourceTracker might not be able to be removed immediately.")
	flag.BoolVar(&RevisionOptimizer.DisableAllComponentRevision, "optimize-disable-component-revision", false, "Optimize ComponentRevision by disabling the creation and gc. Side effect: rollout cannot be used.")
	flag.BoolVar(&RevisionOptimizer.DisableAllApplicationRevision, "optimize-disable-application-revision", false, "Optimize ApplicationRevision by disabling the creation and gc. Side effect: application cannot rollback.")
	flag.BoolVar(&WorkflowOptimizer.DisableRecorder, "optimize-disable-workflow-recorder", false, "Optimize workflow recorder by disabling the creation and gc. Side effect: workflow will not record application after finished running.")
	flag.BoolVar(&WorkflowOptimizer.EnableInMemoryContext, "optimize-enable-in-memory-workflow-context", false, "Optimize workflow by use in-memory context. Side effect: controller crash will lead to mistakes in workflow inputs/outputs.")
	flag.BoolVar(&WorkflowOptimizer.DisableResourceApplyDoubleCheck, "optimize-disable-resource-apply-double-check", false, "Optimize workflow by ignoring resource double check after apply. Side effect: controller will not wait for resource creation.")
	flag.BoolVar(&ResourceTrackerOptimizer.EnableDeleteOnlyTrigger, "optimize-enable-delete-only-trigger", false, "Optimize resourcetracker by only trigger reconcile when resourcetracker is deleted. Side effect: manually non-deletion operation (such as update) on resourcetracker will be ignored.")
}
