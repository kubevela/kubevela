output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: {
		selector: matchLabels: {
			"app.oam.dev/component":   context.name
			"app.oam.dev/appRevision": context.appRevision
		}

		template: {
			metadata: labels: {
				"app.oam.dev/component":   context.name
				"app.oam.dev/appRevision": context.appRevision
			}

			spec: {
				containers: [{
					name:  context.name
					image: parameter.image

					if parameter["cmd"] != _|_ {
						command: parameter.cmd
					}
				}]
			}
		}
	}
}

parameter: {
	// +usage=Which image would you like to use for your service
	// +short=i
	image: string
	// +usage=Commands to run in the container
	cmd?: [...string]
}
