import "vela/test"

_web: {
	definition: "container-image"
	context: name: "web"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [
			{name: "web", image: "shop:1.0", imagePullPolicy: "IfNotPresent"},
			{name: "proxy", image: "envoy:1.30"},
		]
	}
}

"sets the image of the container named after the component": test.#TraitRender & _web & {
	parameter: image: "shop:2.0"
	expect: output: spec: template: spec: containers: [
		{name: "web", image: "shop:2.0", imagePullPolicy: "IfNotPresent"},
		{name: "proxy", image: "envoy:1.30", imagePullPolicy?: _|_},
	]
}

"imagePullPolicy is set when given": test.#TraitRender & _web & {
	parameter: {image: "shop:2.0", imagePullPolicy: "Always"}
	expect: output: spec: template: spec: containers: [{name: "web", image: "shop:2.0", imagePullPolicy: "Always"}, ...]
}

"containerName targets another container": test.#TraitRender & _web & {
	parameter: {containerName: "proxy", image: "envoy:1.31"}
	expect: output: spec: template: spec: containers: [
		{name: "web", image: "shop:1.0"},
		{name: "proxy", image: "envoy:1.31"},
	]
}

"containers sets each named container": test.#TraitRender & _web & {
	parameter: containers: [
		{containerName: "web", image: "shop:2.0"},
		{containerName: "proxy", image: "envoy:1.31", imagePullPolicy: "Never"},
	]
	expect: output: spec: template: spec: containers: [
		{name: "web", image: "shop:2.0", imagePullPolicy: "IfNotPresent"},
		{name: "proxy", image: "envoy:1.31", imagePullPolicy: "Never"},
	]
}

"an image is required": test.#TraitRender & _web & {
	parameter: imagePullPolicy: "Always"
	expect: error:              =~"image"
}

"imagePullPolicy must be one Kubernetes knows": test.#TraitRender & _web & {
	parameter: {image: "shop:2.0", imagePullPolicy: "Sometimes"}
	expect: error: =~"imagePullPolicy"
}

"a container that does not exist is an error": test.#TraitRender & _web & {
	parameter: {containerName: "worker", image: "worker:1.0"}
	expect: error: user: [=~"container worker not found"]
}

"each of several containers must be named": test.#TraitRender & _web & {
	parameter: containers: [{image: "worker:1.0"}]
	expect: error: user: [=~"containerName must be set for containers"]
}
