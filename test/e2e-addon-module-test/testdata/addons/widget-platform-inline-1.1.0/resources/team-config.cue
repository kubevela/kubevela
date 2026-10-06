// NO package header: rendered on its own (with parameter + context), its
// `output` becomes a component appended after the template ones. The name
// comes from `name` below (else from the file name). It is in the module
// component's dependsOn.
output: {
	name: "team-config"
	type: "k8s-objects"
	dependsOn: ["widget-platform-inline-resources"]
	properties: objects: [{
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name:      "widget-platform-inline-team"
			namespace: "widget-platform-inline-system"
		}
		data: {
			team:   parameter.team
			source: "resources/team-config.cue (no package header)"
		}
	}]
}
