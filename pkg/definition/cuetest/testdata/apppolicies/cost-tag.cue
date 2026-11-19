"cost-tag": {
	type:        "policy"
	description: "Labels the Application with its owner"
	attributes: scope: "Application"
}
template: {
	parameter: owner: string
	output: labels: "platform.io/owner": parameter.owner
}
