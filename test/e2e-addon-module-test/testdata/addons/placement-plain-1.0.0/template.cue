package main

// No topology policy: the installer places this addon with the type: addon
// component's placement, and the imported module follows it.
output: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	spec: components: []
}
