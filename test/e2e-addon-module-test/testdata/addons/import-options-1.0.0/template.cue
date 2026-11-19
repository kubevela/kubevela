// The addon author already uses the component name "widget-kit" for
// something that is NOT a module. The generated module component therefore
// gets the next free name, widget-kit-2 (uniqueImportedComponentName).
package main

output: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	spec: components: [{
		name: "widget-kit"
		type: "k8s-objects"
		properties: objects: [{
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {
				name:      "import-options-marker"
				namespace: "vela-system"
			}
			data: note: "this component is not a module; it only takes the name widget-kit"
		}]
	}]
}
