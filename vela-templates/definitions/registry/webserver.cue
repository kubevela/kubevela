webserver: {
	type: "component"
	annotations: {}
	labels: {}
	description: "webserver was composed by deployment and service"
	attributes: workload: definition: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
	}
}
template: {
	output: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		spec: {
			selector: matchLabels: "app.oam.dev/component": context.name
			template: {
				metadata: labels: "app.oam.dev/component": context.name
				spec: containers: [{
					name:  context.name
					image: parameter.image

					if parameter["cmd"] != _|_ {
						command: parameter.cmd
					}

					if parameter["env"] != _|_ {
						env: parameter.env
					}

					if context["config"] != _|_ {
						env: context.config
					}

					ports: [{
						containerPort: parameter.port
					}]

					if parameter["cpu"] != _|_ {
						resources: {
							limits: cpu:   parameter.cpu
							requests: cpu: parameter.cpu
						}
					}
				}]
			}
		}
	}
	// workload can have extra object composition by using 'outputs' keyword
	outputs: service: {
		apiVersion: "v1"
		kind:       "Service"
		spec: {
			selector: "app.oam.dev/component": context.name
			ports: [
				{
					port:       parameter.port
					targetPort: parameter.port
				},
			]
		}
	}
	parameter: {
		image: string
		cmd?: [...string]
		port: *80 | int
		env?: [...{
			name:   string
			value?: string
			valueFrom?: secretKeyRef: {
				name: string
				key:  string
			}
		}]
		cpu?: string
	}
}
