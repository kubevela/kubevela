tuned: {
	type:        "source"
	description: "Reads the Application's name, cached briefly and never served stale"
}
template: {
	schema: {app: string}
	storage: {
		storageTTL:     "90s"
		onStaleFailure: "fail"
	}
	parameter: {}
	output: app: context.appName
}
