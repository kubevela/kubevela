import "vela/multicluster"

places: {
	type:        "workflow-step"
	description: "Reads where the Application's topology places it"
}
template: {
	placements: multicluster.#GetPlacementsFromTopologyPolicies & {$params: policies: []}
	parameter: {}
}
