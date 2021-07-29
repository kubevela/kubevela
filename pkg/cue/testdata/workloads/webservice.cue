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
				if parameter.addRevisionLabel {
					"app.oam.dev/appRevision": context.appRevision
				}
			}
			spec: {
				containers: [{
					name:  context.name
					image: parameter.image
					if parameter["env"] != _|_ {
						env: parameter.env
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
	// +usage=Define arguments by using environment variables
	env?: [...{
		// +usage=Environment variable name
		name: string
		// +usage=The value of the environment variable
		value?: string
		// +usage=Specifies a source the value of this var should come from
		valueFrom?: {
			// +usage=Selects a key of a secret in the pod's namespace
			secretKeyRef: {
				// +usage=The name of the secret in the pod's namespace to select from
				name: string
				// +ignore
				// +usage=The key of the secret to select from. Must be a valid secret key
				secretKey: string
			}
		}
	}]
  // +ignore
	// +usage=If addRevisionLabel is true, the appRevision label will be added to the underlying pods
	addRevisionLabel: *false | bool
}

