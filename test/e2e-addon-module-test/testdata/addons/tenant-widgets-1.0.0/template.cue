// A hand-written type: module component. This is the only way to pass the
// module component's `namespace` property from an addon, because
// modules/_imports.cue has no such field. Because a component of type module
// for widget-kit already exists here, the matching _imports.cue entry is
// skipped without a warning (existingModuleNames).
//
// Hand-written module components get no automatic dependsOn, so the
// dependency on the namespace component is spelled out.
package main

output: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	spec: components: [{
		name: "tenant-kit"
		type: "module"
		dependsOn: ["tenant-widgets-resources"]
		properties: {
			module:    "widget-kit"
			registry:  "e2e-modules"
			version:   "1.0.0"
			namespace: "kit-tenant"
		}
	}]
}
