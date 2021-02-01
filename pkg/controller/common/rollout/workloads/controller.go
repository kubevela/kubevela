package workloads

// WorkloadController is the interface that all type of workload controller implements
type WorkloadController interface {
	// Initialize makes sure that the resources can be upgraded according to the rollout plan
	// it returns the number of available pods that are upgrade (with the new spec)
	Initialize() (int32, error)

	// RolloutPods tries to upgrade pods in the resources following the rollout plan
	// it will upgrade as many pods as the rollout plan allows at once, the routine does not block on any operations.
	// Instead, we rely on the go-client's requeue mechanism to drive this towards the spec goal
	// it returns the number of pods upgraded in this round
	RolloutPods() (int32, error)

	/*
		GetMetadata() (string, map[string]int32, error)

		SyncStatus() error

		SetStatusFailedChecks() error

		ScaleToZero() error
	*/
	// Finalize makes sure the resources are in a good final state.
	// For example, we may remove the source object to prevent scalar traits to ever work
	// or we may add an annotation to indicate the upgrade finished time
	Finalize() error
}
