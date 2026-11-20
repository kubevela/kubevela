// A PolicyDefinition, to show that a line may ship definition kinds other
// than components and traits. Installs as "demo-store-v2-retention" and is
// referenced from an Application's policies list.
//
// Supported kinds are component, trait, policy, workflow-step and workload.
"retention": {
	type: "policy"
	description: "Declares how long this application's buckets retain data."
}

template: {
	// A policy that only carries configuration needs no output; the template
	// block still has to exist and be non-empty, so parameter alone is what
	// it holds.
	parameter: {
		// Retention window in days.
		days: *30 | int
	}
}
