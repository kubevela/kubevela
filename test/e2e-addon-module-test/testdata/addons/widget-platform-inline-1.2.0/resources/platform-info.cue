// `package main`: merged into template.cue's build, referenced there as
// platformInfo. Not rendered as a separate component by RenderResources.
package main

platformInfo: {
	name: "platform-info"
	type: "k8s-objects"
	dependsOn: ["widget-platform-inline-resources"]
	properties: objects: [{
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name:      "widget-platform-inline-info"
			namespace: "widget-platform-inline-system"
		}
		data: {
			greeting:     parameter.greeting
			addonVersion: context.metadata.version
			source:       "resources/platform-info.cue (package main)"
		}
	}]
}
