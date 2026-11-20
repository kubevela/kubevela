// Minimal addon template: the Application only carries what resources/ and
// modules/_imports.cue add to it.
package main

output: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	spec: components: []
}
