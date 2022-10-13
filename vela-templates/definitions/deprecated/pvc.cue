pvc: {
	type: "trait"
	annotations: {}
	labels: {
		"deprecated": "true"
	}
	description: "Create a Persistent Volume Claim and mount the PVC as volume to the  first container in the pod. This definition is DEPRECATED, please specify pvc in 'storage' instead."
	attributes: {
		appliesToWorkloads: ["deployments.apps"]
		podDisruptive: true
	}
}
template: {
	patch: spec: template: spec: {
		containers: [{
			if parameter.volumeMode == "Block" {
				// +patchKey=name
				volumeDevices: [
					for v in parameter.volumesToMount {
						{
							name:       v.name
							devicePath: v.devicePath
						}
					},
				]
			}
			if parameter.volumeMode == "Filesystem" {
				// +patchKey=name
				volumeMounts: [
					for v in parameter.volumesToMount {
						{
							name:      v.name
							mountPath: v.mountPath
						}
					},
				]
			}
		}]

		// +patchKey=name
		volumes: [
			for v in parameter.volumesToMount {
				{
					name: v.name
					persistentVolumeClaim: claimName: parameter.claimName
				}
			},
		]
	}
	outputs: "claim": {
		apiVersion: "v1"
		kind:       "PersistentVolumeClaim"
		metadata: {
			name: parameter.claimName
		}
		spec: {
			accessModes: parameter.accessModes
			volumeMode:  parameter.volumeMode
			if parameter.volumeName != _|_ {
				volumeName: parameter.volumeName
			}

			if parameter.storageClassName != _|_ {
				storageClassName: parameter.storageClassName
			}
			resources: requests: storage: parameter.resources.requests.storage
			if parameter.resources.limits != _|_ {
				resources: limits: storage: parameter.resources.limits.storage
			}
			if parameter.dataSourceRef != _|_ {
				dataSourceRef: parameter.dataSourceRef
			}
			if parameter.dataSource != _|_ {
				dataSource: parameter.dataSource
			}
			if parameter.selector != _|_ {
				dataSource: parameter.selector
			}
		}
	}
	parameter: {
		claimName:   string
		volumeMode:  *"Filesystem" | string
		volumeName?: string
		accessModes: [...string]
		storageClassName?: string
		resources: {
			requests: storage: =~"^([1-9][0-9]{0,63})(E|P|T|G|M|K|Ei|Pi|Ti|Gi|Mi|Ki)$"
			limits?: storage:  =~"^([1-9][0-9]{0,63})(E|P|T|G|M|K|Ei|Pi|Ti|Gi|Mi|Ki)$"
		}
		dataSourceRef?: {
			name:     string
			kind:     string
			apiGroup: string
		}
		dataSource?: {
			name:     string
			kind:     string
			apiGroup: string
		}
		selector?: {
			matchLabels?: [string]: string
			matchExpressions?: {
				key: string
				values: [...string]
				operator: string
			}
		}
		volumesToMount: [...{
			name: string
			if volumeMode == "Block" {
				devicePath: string
			}
			if volumeMode == "Filesystem" {
				mountPath: string
			}
		}]
	}
}
