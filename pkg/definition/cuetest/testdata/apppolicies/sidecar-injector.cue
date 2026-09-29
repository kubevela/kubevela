"sidecar-injector": {
	type:        "policy"
	description: "Adds a logging sidecar to every component, unless the Application opts out"
	attributes: scope: "Application"
}
template: {
	parameter: {
		image: *"fluent-bit:3.0" | string
	}
	enabled: context.appAnnotations["logging.io/opt-out"] == _|_
	output: {
		components: [for c in context.appComponents {
			{for k, v in c if k != "traits" {(k): v}}
			traits: [
				for t in *c.traits | [] {t},
				{type: "sidecar", properties: {name: "logger", image: parameter.image}},
			]
		}]
		annotations: "logging.io/injected-by": context.policyName
		ctx: logging: sidecar: parameter.image
	}
}
