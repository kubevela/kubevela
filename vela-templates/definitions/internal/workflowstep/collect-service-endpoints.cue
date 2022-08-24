import (
	"vela/op"
	"vela/ql"
)

"collect-service-endpoints": {
	type: "workflow-step"
	annotations: {}
	labels: {}
	description: "Collect service endpoints for the application."
}
template: {
	collect: ql.#CollectServiceEndpoints & {
		app: {
			name:      *context.name | string
			namespace: *context.namespace | string
			if parameter.name != _|_ {
				name: parameter.name
			}
			if parameter.namespace != _|_ {
				namespace: parameter.namespace
			}
			filter: {
				if parameter.components != _|_ {
					components: parameter.components
				}
			}
		}
	} @step(1)

	outputs: {
		eps: *[] | [...]
		if parameter.port == _|_ {
			eps: collect.list
		}
		if parameter.port != _|_ {
			eps: [ for ep in collect.list if parameter.port == ep.endpoint.port {ep}]
		}
		endpoints: *[] | [...]
		if parameter.outer != _|_ {
			tmps: [ for ep in eps {
				ep
				if ep.endpoint.inner == _|_ {
					outer: true
				}
				if ep.endpoint.inner != _|_ {
					outer: !ep.endpoint.inner
				}
			}]
			endpoints: [ for ep in tmps if (!parameter.outer || ep.outer) {ep}]
		}
		if parameter.outer == _|_ {
			endpoints: eps
		}
	}

	wait: op.#ConditionalWait & {
		continue: len(outputs.endpoints) > 0
	} @step(2)

	value: {
		if len(outputs.endpoints) > 0 {
			outputs.endpoints[0]
		}
	}

	parameter: {
		// +usage=Specify the name of the application
		name?: string
		// +usage=Specify the namespace of the application
		namespace?: string
		// +usage=Filter the component of the endpoints
		components?: [...string]
		// +usage=Filter the port of the endpoints
		port?: int
		// +usage=Filter the endpoint that are only outer
		outer?: bool
	}
}
