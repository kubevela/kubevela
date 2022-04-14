storageclass: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Add storageclass on K8s pod for your workload which follows the pod spec in path 'spec.template'."
	attributes: {
		appliesToWorkloads: ["*"]
	}
}
template: {

	volumeClaimTemplatesList: *[
					for v in parameter.volumeClaimTemplates {
			{
				metadata: name: v.name
				spec: {
					accessModes: ["ReadWriteOnce"]
					resources: requests: storage: v.requests
					storageClassName: v.storageClassName
				}
			}
		},
	] | []

	volumeClaimTemplateVolumeMountsList: *[
						for v in parameter.volumeClaimTemplates {
			{
				name:      v.name
				mountPath: v.mountPath
			}
		},
	] | []

	patch: {
		// +patchKey=name
		spec: {
			template: spec: {
				containers: [...{
					// +patchKey=name
					volumeMounts: volumeClaimTemplateVolumeMountsList
				}]
			}
			// +patchKey=name
			volumeClaimTemplates: volumeClaimTemplatesList
		}
	}

	parameter: {
		volumeClaimTemplates?: [...{
			name:             string
			requests:         string
			storageClassName: string
			mountPath:        string
		}]
	}
}
