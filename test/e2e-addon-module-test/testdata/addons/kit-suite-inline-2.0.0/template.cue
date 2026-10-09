// Minimal addon template: the Application only carries what resources/ and
// the inline module under modules/ add to it.
package main

output: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	spec: components: []
}
