// workflow-check-global-replica.cue
//
// Demonstrates upgrade rule 3: bool-default-negation
//
// CUE v0.14 regressed evaluation of `bool | *false` in if-guards. The default
// value is read before unification, so `if !_isSecondary` fires even when
// a caller has set _isSecondary to true. This causes secondary replicas to
// incorrectly fail the engineVersion requirement check.
//
// The upgrade engine rewrites the flag declaration so the boolean value is
// derived from the conditions directly, rather than from a default:
//
// Before (broken in CUE v0.14+):
//   _isSecondary: bool | *false
//   if parameter.globalCluster != _|_ {
//       if parameter.globalCluster.mode == "secondary" {
//           _isSecondary: true
//       }
//   }
//   if !_isSecondary {
//       if parameter.engineVersion == _|_ {
//           _error: 0 & "engineVersion is required for primary clusters"
//       }
//   }
//
// After upgrade (direct expression strategy):
//   _isSecondary: (parameter.globalCluster != _|_ && parameter.globalCluster.mode == "secondary")
//   if !_isSecondary {
//       if parameter.engineVersion == _|_ {
//           _error: 0 & "engineVersion is required for primary clusters"
//       }
//   }

"check-global-replica": {
	type:        "workflow-step"
	description: "Validates global cluster replica configuration before deployment. Requires engineVersion for primaries; skips the check for secondaries."
}

template: {
	// ISSUE 3 (bool-default-negation): in CUE v0.14+ the evaluator reads the
	// default of `bool | *false` before unification, so `if !_isSecondary`
	// incorrectly fires for secondary clusters. The upgrade engine inlines the
	// condition as a direct boolean expression to fix evaluation order.
	_isSecondary: bool | *false
	if parameter.globalCluster != _|_ {
		if parameter.globalCluster.mode == "secondary" {
			_isSecondary: true
		}
	}

	// This guard must NOT fire for secondary clusters.
	if !_isSecondary {
		if parameter.engineVersion == _|_ {
			// Triggers a CUE unification error, surfaced as a workflow step failure.
			_engineVersionRequired: 0 & "engineVersion is required for primary clusters"
		}
	}

	outputs: "replica-check-result": {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name:      "replica-check-result"
			namespace: context.namespace
		}
		data: {
			isSecondary: "\(_isSecondary)"
			if parameter.globalCluster != _|_ {
				clusterMode: parameter.globalCluster.mode
			}
			if parameter.engineVersion != _|_ {
				engineVersion: parameter.engineVersion
			}
		}
	}

	parameter: {
		// +usage=Global cluster configuration. Omit for standalone deployments.
		globalCluster?: {
			// +usage=Cluster role: primary or secondary
			mode: "primary" | "secondary"
			// +usage=Cluster identifier
			id: string
		}

		// +usage=Database engine version. Required for primary clusters.
		engineVersion?: string
	}
}
