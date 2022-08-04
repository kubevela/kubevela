parameter: {
	// +usage=Which image would you like to use for your service
	// +short=i
	image: string

	// +usage=Commands to run in the container
	cmd?: [...string]

	cpu?: string
}

#routeName: "\(context.appName)-\(context.name)"

context: {}
