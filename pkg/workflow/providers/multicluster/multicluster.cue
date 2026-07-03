// multicluster.cue

#ListClusters: {
	#provider: "multicluster"
	#do:       "list-clusters"

	$returns?: {
		outputs: {
			clusters: [...string]
		}
	}
}

#GetPlacementsFromTopologyPolicies: {
	#provider: "multicluster"
	#do:       "get-placements-from-topology-policies"

	$params: {
		policies: [...string]
	}
	$returns?: {
		placements: [...{
			cluster:   string
			namespace: string
		}]
	}
}

#Deploy: {
	#provider: "multicluster"
	#do:       "deploy"

	$params: {
		policies: [...string]
		parallelism:              int
		ignoreTerraformComponent: bool
		inlinePolicies: *[] | [...{...}]
	}
	$returns?: {...}
}
