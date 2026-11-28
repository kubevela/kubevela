import "vela/test"

_rendered: $returns: application: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	metadata: {
		name:      "crate-widget"
		namespace: "vela-system"
	}
}

"the crate Application is the output": test.#ComponentRender & {
	definition: "crate"
	context: {
		name:      "widget"
		namespace: "team-a"
	}
	mocks: "vela/crate": "#Render": _rendered
	expect: {
		output: {kind: "Application", metadata: name: "crate-widget"}
		calls: "vela/crate": "#Render": [{$params: {
			crate:     "widget"
			registry:  ""
			namespace: "team-a"
			version:   ""
		}}]
	}
}

"parameters pass through to crate render": test.#ComponentRender & {
	definition: "crate"
	context: {
		name:      "ignored"
		namespace: "team-a"
	}
	parameter: {
		crate:    "widget-kit"
		registry: "internal"
		version:  "1.2.3"
	}
	mocks: "vela/crate": "#Render": _rendered
	expect: calls: "vela/crate": "#Render": [{$params: {
		crate:     "widget-kit"
		registry:  "internal"
		namespace: "team-a"
		version:   "1.2.3"
	}}]
}

"an owned addon Application installs into the recorded namespace": test.#ComponentRender & {
	definition: "crate"
	context: {
		name:      "widget"
		namespace: "vela-system"
		appLabels: "addons.oam.dev/name":                   "tenant-widgets"
		appAnnotations: "crates.oam.dev/install-namespace": "kit-tenant"
	}
	mocks: "vela/crate": "#Render": _rendered
	expect: calls: "vela/crate": "#Render": [{$params: namespace: "kit-tenant", ...}]
}

"the recorded namespace is ignored on a user Application": test.#ComponentRender & {
	definition: "crate"
	context: {
		name:      "widget"
		namespace: "team-a"
		appAnnotations: "crates.oam.dev/install-namespace": "elsewhere"
	}
	mocks: "vela/crate": "#Render": _rendered
	expect: calls: "vela/crate": "#Render": [{$params: namespace: "team-a", ...}]
}

"placement clusters pass through to crate render": test.#ComponentRender & {
	definition: "crate"
	context: {
		name:      "widget"
		namespace: "team-a"
	}
	parameter: placement: {
		clusters: ["spoke"]
		allowEmpty: true
	}
	mocks: "vela/crate": "#Render": _rendered
	expect: calls: "vela/crate": "#Render": [{$params: placement: {
		clusters: ["spoke"]
		allowEmpty: true
	}, ...
	}]
}

"placement clusterLabelSelector passes through to crate render": test.#ComponentRender & {
	definition: "crate"
	context: {
		name:      "widget"
		namespace: "team-a"
	}
	parameter: placement: clusterLabelSelector: tier: "runtime"
	mocks: "vela/crate": "#Render": _rendered
	expect: calls: "vela/crate": "#Render": [{$params: placement: clusterLabelSelector: tier: "runtime", ...}]
}

// The crate provider rejects an empty placement and one that sets both
// fields. The template hands either one to the provider unchanged.
"an empty placement reaches the provider unchanged": test.#ComponentRender & {
	definition: "crate"
	context: {
		name:      "widget"
		namespace: "team-a"
	}
	parameter: placement: {}
	mocks: "vela/crate": "#Render": _rendered
	expect: calls: "vela/crate": "#Render": [{$params: placement: {}, ...}]
}

"clusters and a selector together reach the provider unchanged": test.#ComponentRender & {
	definition: "crate"
	context: {
		name:      "widget"
		namespace: "team-a"
	}
	parameter: placement: {
		clusters: ["spoke"]
		clusterLabelSelector: tier: "runtime"
	}
	mocks: "vela/crate": "#Render": _rendered
	expect: calls: "vela/crate": "#Render": [{$params: placement: {
		clusters: ["spoke"]
		clusterLabelSelector: tier: "runtime"
	}, ...
	}]
}

"a namespace property is refused": test.#ComponentRender & {
	definition: "crate"
	context: {
		name:      "widget"
		namespace: "team-a"
	}
	parameter: namespace: "acme-system"
	mocks: "vela/crate": "#Render": _rendered
	expect: error: =~"namespace is no longer accepted"
}
