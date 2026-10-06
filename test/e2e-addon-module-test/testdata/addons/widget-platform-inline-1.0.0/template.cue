// The addon Application. `package main` pulls in every resources/*.cue file
// that also declares `package main` (here: platform-info.cue).
//
// Components that come from THIS file are not part of the dependsOn list the
// renderer gives the generated type: module component. Only resources/ files
// rendered as their own components (YAML files, and .cue files WITHOUT a
// package header) and the `outputs` block are waited for. See architecture.md
// section 5.3.
package main

output: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	// name/namespace are always forced to addon-widget-platform-inline/vela-system.
	spec: components: [platformInfo]
}

// Auxiliary objects. With type: addon they are wrapped into a k8s-objects
// component called addon-auxiliaries, which the module component depends on.
outputs: "platform-notes": {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: {
		name:      "widget-platform-inline-notes"
		namespace: "vela-system"
	}
	data: {
		greeting:     parameter.greeting
		addonVersion: context.metadata.version
	}
}
