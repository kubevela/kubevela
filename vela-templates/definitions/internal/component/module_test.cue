import "vela/test"

_rendered: $returns: application: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	metadata: {
		name:      "module-widget"
		namespace: "vela-system"
	}
}

"the module Application is the output": test.#ComponentRender & {
	definition: "module"
	context: name: "widget"
	mocks: "vela/module": "#Render": _rendered
	expect: {
		output: {kind: "Application", metadata: name: "module-widget"}
		calls: "vela/module": "#Render": [{$params: {
			module:    "widget"
			registry:  ""
			namespace: ""
			version:   ""
		}}]
	}
}

"parameters pass through to module render": test.#ComponentRender & {
	definition: "module"
	context: name: "ignored"
	parameter: {
		module:    "widget-kit"
		registry:  "internal"
		namespace: "acme-system"
		version:   "1.2.3"
	}
	mocks: "vela/module": "#Render": _rendered
	expect: calls: "vela/module": "#Render": [{$params: {
		module:    "widget-kit"
		registry:  "internal"
		namespace: "acme-system"
		version:   "1.2.3"
	}}]
}
