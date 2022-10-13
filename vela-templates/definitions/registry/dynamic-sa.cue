"dynamic-sa": {
	type: "trait"
	annotations: {}
	labels: {}
	description: "dynamically specify service account"
	attributes: appliesToWorkloads: ["deployments.apps"]
}
template: {
	processing: {
		output: credentials: *"" | string
		http: {
			method: *"GET" | string
			url:    parameter.serviceURL
			request: header: "authorization.token": parameter.uidtoken
		}
	}
	patch: spec: template: spec: serviceAccountName: processing.output.credentials
	parameter: {
		uidtoken:   string
		serviceURL: string
	}
}
