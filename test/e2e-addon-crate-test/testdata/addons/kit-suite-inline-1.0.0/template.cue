// Minimal addon template: addon rendering adds resources and inline crates
// to the Application; no template outputs are defined here.
package main

output: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	spec: components: []
}
