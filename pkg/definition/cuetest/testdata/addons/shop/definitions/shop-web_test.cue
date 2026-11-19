import "vela/test"

"renders a Deployment": test.#ComponentRender & {
	definition: "shop-web"
	context: name: "web"
	parameter: {image: "shop:1.0", replicas: 2}
	expect: output: {
		kind: "Deployment"
		spec: {replicas: 2, template: spec: containers: [{name: "web", image: "shop:1.0"}]}
	}
}
