import "vela/test"

// Rendering an addon fetches it from a registry, so the render is mocked.
_rendered: $returns: {
	resolvedVersion: "2.3.0"
	registry:        "KubeVela"
	application: {apiVersion: "core.oam.dev/v1beta1", kind: "Application", metadata: name: "addon-fluxcd"}
}

"the addon's Application is the output": test.#ComponentRender & {
	definition: "addon"
	context: {
		name:      "fluxcd"
		namespace: "team-a"
	}
	mocks: "vela/addon": "#Render": _rendered
	expect: {
		output: {kind: "Application", metadata: name: "addon-fluxcd"}
		// The addon is the component's name, latest version from the
		// default registry, its checks enforced.
		calls: "vela/addon": "#Render": [{$params: {
			addon:    "fluxcd"
			version:  ""
			registry: ""
			properties: {} @exact()
			skipVersionValidate: false
			// The installing Application's namespace, where the addon's
			// modules install their definitions.
			namespace: "team-a"
		}}]
	}
}

"parameters pass through to the render": test.#ComponentRender & {
	definition: "addon"
	context: {
		name:      "gitops"
		namespace: "team-a"
	}
	parameter: {
		addon:    "fluxcd"
		version:  "2.3.0"
		registry: "internal"
		properties: {onlyHelmComponents: true}
		skipVersionValidation: true
	}
	mocks: "vela/addon": "#Render": _rendered
	expect: calls: "vela/addon": "#Render": [{$params: {
		addon:    "fluxcd"
		version:  "2.3.0"
		registry: "internal"
		properties: onlyHelmComponents: true
		skipVersionValidate: true
		namespace:           "team-a"
	}}]
}

"placement passes through to the render": test.#ComponentRender & {
	definition: "addon"
	context: {
		name:      "platform"
		namespace: "team-a"
	}
	parameter: placement: {
		clusterLabelSelector: tier: "runtime"
		allowEmpty: true
	}
	mocks: "vela/addon": "#Render": _rendered
	expect: calls: "vela/addon": "#Render": [{$params: placement: {
		clusterLabelSelector: tier: "runtime"
		allowEmpty: true
	}, ...
	}]
}

"no placement leaves the render call unchanged": test.#ComponentRender & {
	definition: "addon"
	context: {
		name:      "fluxcd"
		namespace: "team-a"
	}
	mocks: "vela/addon": "#Render": _rendered
	expect: calls: "vela/addon": "#Render": [{$params: {
		addon:    "fluxcd"
		version:  ""
		registry: ""
		properties: {} @exact()
		skipVersionValidate: false
		namespace:           "team-a"
	} @exact()
	}]
}

"a cluster list passes through to the render": test.#ComponentRender & {
	definition: "addon"
	context: {
		name:      "platform"
		namespace: "team-a"
	}
	parameter: placement: clusters: ["spoke-a", "spoke-b"]
	mocks: "vela/addon": "#Render": _rendered
	expect: calls: "vela/addon": "#Render": [{$params: {
		placement: {
			clusters: ["spoke-a", "spoke-b"]
		} @exact()
	}, ...
	}]
}
