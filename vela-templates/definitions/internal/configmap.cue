configmap: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Create/Attach configmaps to workloads."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps"]
	}
}
template: {
	patch: {
		spec: template: spec: {
			containers: [{
				volumeMounts: [
					// +patchKey=name
					for v in parameter.volumes {
						{
							name:      "volume-\(v.name)"
							mountPath: v.mountPath
							readOnly:  v.readOnly
						}
					},
				]
			}, ...]
			volumes: [
				// +patchKey=name
				for v in parameter.volumes {
					{
						name: "volume-\(v.name)"
						configMap: name: v.name
					}
				},
			]
		}
	}
	outputs: {
		for v in parameter.volumes {
			if v.data != _|_ {
				"\(v.name)": {
					apiVersion: "v1"
					kind:       "ConfigMap"
					metadata: name: v.name
					data: v.data
				}
			}
		}
	}
	parameter: {
		// +usage=Specify mounted configmap names and their mount paths in the container
		volumes: [...{
			name:      string
			mountPath: string
			readOnly:  *false | bool
			data?: [string]: string
		}]
	}
}
