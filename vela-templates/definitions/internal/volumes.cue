volumes: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Add volumes for your Workload."
	attributes: {
		appliesToWorkloads: ["deployments.apps"]
		podDisruptive: true
	}
}
template: {
	patch: {
		spec: template: spec: volumes: [
			for v in parameter.volumes {
				{
					name: v.name
					if v.type == "pvc" {
						persistentVolumeClaim: {
							claimName: v.claimName
						}
					}
					if v.type == "configMap" {
						configMap: {
							defaultMode: v.defaultMode
							name:        v.cmName
							if v.items != _|_ {
								items: v.items
							}
						}
					}
					if v.type == "secret" {
						secret: {
							defaultMode: v.defaultMode
							secretName:  v.secretName
							if v.items != _|_ {
								items: v.items
							}
						}
					}
					if v.type == "emptyDir" {
						emptyDir: {
							medium: v.medium
						}
					}
				}
			},
		]
	}

	parameter: {
		// +usage=Declare volumes and volumeMounts
		volumes?: [...{
			name: string
			// +usage=Specify volume type, options: "pvc","configMap","secret","emptyDir"
			type: "pvc" | "configMap" | "secret" | "emptyDir"
			if type == "pvc" {
				claimName: string
			}
			if type == "configMap" {
				defaultMode: *420 | int
				cmName:      string
				items?: [...{
					key:  string
					path: string
					mode: *511 | int
				}]
			}
			if type == "secret" {
				defaultMode: *420 | int
				secretName:  string
				items?: [...{
					key:  string
					path: string
					mode: *511 | int
				}]
			}
			if type == "emptyDir" {
				medium: *"" | "Memory"
			}
		}]
	}

}
