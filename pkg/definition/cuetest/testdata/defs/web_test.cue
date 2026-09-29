import "vela/test"

"defaults to one replica": test.#ComponentRender & {
	definition: "web"
	parameter: image: "nginx"
	expect: {
		output: spec: replicas: 1
		outputs: service?: _|_
	}
}

"exposes a service": test.#ComponentRender & {
	definition: "web"
	context: name: "api"
	parameter: {image: "nginx", expose: true}
	expect: outputs: service: metadata: name: "api"
}

"rejects a numeric image": test.#ComponentRender & {
	definition: "web"
	parameter: image: 3
	expect: error: =~"image"
}

"fails on purpose": test.#ComponentRender & {
	definition: "web"
	parameter: image: "nginx"
	expect: output: spec: replicas: 2
}

"expects an error that never comes": test.#ComponentRender & {
	definition: "web"
	parameter: image: "nginx"
	expect: error: =~"image"
}

"healthy once ready": test.#ComponentStatus & {
	definition: "web"
	parameter: {image: "nginx", replicas: 2}
	observed: output: status: readyReplicas: 2
	expect: {
		healthy: true
		message: "Ready:2/2"
		details: image: "nginx"
	}
}

"unhealthy while rolling out": test.#ComponentStatus & {
	definition: "web"
	parameter: {image: "nginx", replicas: 2}
	observed: output: status: readyReplicas: 1
	expect: {healthy: false, message: "Ready:1/2"}
}

"status errors before any status exists": test.#ComponentStatus & {
	definition: "web"
	parameter: image: "nginx"
	expect: healthy: false
}

_notACase: {definition: "web"}

"closed on purpose": test.#ComponentRender & {
	definition: "web"
	parameter: image: "nginx"
	expect: output: spec: {replicas: 1} @exact()
}
