team: {
	type: "trait"
	attributes: {
		appliesToWorkloads: ["*"]
		status: {
			healthPolicy: #"""
				isHealth: *(context.outputs.cfg.metadata.annotations.synced == "true") | false
				"""#
			customStatus: #"""
				message: "team \(parameter.team)"
				"""#
		}
	}
}
template: {
	patch: metadata: labels: team: parameter.team
	outputs: cfg: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: "\(context.name)-team"
		data: team: parameter.team
	}
	parameter: team: string
}
