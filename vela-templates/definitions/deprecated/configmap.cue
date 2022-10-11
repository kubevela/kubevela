configmap: {
	type: "trait"
	annotations: {}
	labels: {
		"deprecated": "true"
	}
	description: "Create/Attach configmaps on K8s pod for your workload which follows the pod spec in path 'spec.template'. This definition is DEPRECATED, please specify configmap in 'storage' instead."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["*"]
	}
}
template: {
	patch: spec: template: spec: {
		containers: [{
			// +patchKey=name
			volumeMounts: [
				for v in parameter.volumes {
					{
						name:      "volume-\(v.name)"
						mountPath: v.mountPath
						readOnly:  v.readOnly
					}
				},
			]
		}, ...]
		// +patchKey=name
		volumes: [
			for v in parameter.volumes {
				{
					name: "volume-\(v.name)"
					configMap: name: v.name
				}
			},
		]
	}
	outputs: {
		for v in parameter.volumes {
			if v.data != _|_ {
				(v.name): {
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
