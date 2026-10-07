"fleet-agent": {
	type: "trait"
	attributes: appliesToWorkloads: ["*"]
}
template: {
	patch: metadata: labels: "fleet.example.com/agent": "true"
	parameter: {}
}
