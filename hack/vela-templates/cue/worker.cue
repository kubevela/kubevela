output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: {
		selector: matchLabels: {
			"app.oam.dev/component": context.name
		}

		template: {
			metadata: labels: {
				"app.oam.dev/component": context.name
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

		selector:
			matchLabels:
				"app.oam.dev/component": context.name
	}
}

parameter: {
	// +usage=specify app image
	// +short=i
	image: string

	cmd?: [...string]
}
