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
	context: {
		name:      "widget"
		namespace: "team-a"
	}
	mocks: "vela/module": "#Render": _rendered
	expect: {
		output: {kind: "Application", metadata: name: "module-widget"}
		calls: "vela/module": "#Render": [{$params: {
			module:    "widget"
			registry:  ""
			namespace: "team-a"
			version:   ""
		}}]
	}
}

"parameters pass through to module render": test.#ComponentRender & {
	definition: "module"
	context: {
		name:      "ignored"
		namespace: "team-a"
	}
	parameter: {
		module:   "widget-kit"
		registry: "internal"
		version:  "1.2.3"
	}
	mocks: "vela/module": "#Render": _rendered
	expect: calls: "vela/module": "#Render": [{$params: {
		module:    "widget-kit"
		registry:  "internal"
		namespace: "team-a"
		version:   "1.2.3"
	}}]
}

"an owned addon Application installs into the recorded namespace": test.#ComponentRender & {
	definition: "module"
	context: {
		name:      "widget"
		namespace: "vela-system"
		appLabels: "addons.oam.dev/name":                    "tenant-widgets"
		appAnnotations: "modules.oam.dev/install-namespace": "kit-tenant"
	}
	mocks: "vela/module": "#Render": _rendered
	expect: calls: "vela/module": "#Render": [{$params: namespace: "kit-tenant", ...}]
}

"the recorded namespace is ignored on a user Application": test.#ComponentRender & {
	definition: "module"
	context: {
		name:      "widget"
		namespace: "team-a"
		appAnnotations: "modules.oam.dev/install-namespace": "elsewhere"
	}
	mocks: "vela/module": "#Render": _rendered
	expect: calls: "vela/module": "#Render": [{$params: namespace: "team-a", ...}]
}

"placement clusters pass through to module render": test.#ComponentRender & {
	definition: "module"
	context: {
		name:      "widget"
		namespace: "team-a"
	}
	parameter: placement: {
		clusters: ["spoke"]
		allowEmpty: true
	}
	mocks: "vela/module": "#Render": _rendered
	expect: calls: "vela/module": "#Render": [{$params: placement: {
		clusters: ["spoke"]
		allowEmpty: true
	}, ...
	}]
}

"placement clusterLabelSelector passes through to module render": test.#ComponentRender & {
	definition: "module"
	context: {
		name:      "widget"
		namespace: "team-a"
	}
	parameter: placement: clusterLabelSelector: tier: "runtime"
	mocks: "vela/module": "#Render": _rendered
	expect: calls: "vela/module": "#Render": [{$params: placement: clusterLabelSelector: tier: "runtime", ...}]
}

"an empty placement is refused": test.#ComponentRender & {
	definition: "module"
	context: {
		name:      "widget"
		namespace: "team-a"
	}
	parameter: placement: {}
	mocks: "vela/module": "#Render": _rendered
	expect: error: =~"set clusters or clusterLabelSelector"
}

"placement rejects clusters and a selector together": test.#ComponentRender & {
	definition: "module"
	context: {
		name:      "widget"
		namespace: "team-a"
	}
	parameter: placement: {
		clusters: ["spoke"]
		clusterLabelSelector: tier: "runtime"
	}
	mocks: "vela/module": "#Render": _rendered
	expect: error: =~"set clusters or clusterLabelSelector"
}

"a namespace property is refused": test.#ComponentRender & {
	definition: "module"
	context: {
		name:      "widget"
		namespace: "team-a"
	}
	parameter: namespace: "acme-system"
	mocks: "vela/module": "#Render": _rendered
	expect: error: =~"namespace is no longer accepted"
}
