// Minimal addon template: addon rendering adds resources and inline modules
// to the Application; no template outputs are defined here.
package main

output: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	spec: components: []
}
