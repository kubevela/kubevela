storage: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Add storages on K8s pod for your workload which follows the pod spec in path 'spec.template'."
	attributes: {
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch"]
		podDisruptive: true
	}
}
template: {
	pvcVolumesList: *[
			if parameter.pvc != _|_ for v in parameter.pvc {
			{
				name: "pvc-" + v.name
				persistentVolumeClaim: claimName: v.name
			}
		},
	] | []

	configMapVolumesList: *[
				if parameter.configMap != _|_ for v in parameter.configMap if v.mountPath != _|_ {
			{
				name: "configmap-" + v.name
				configMap: {
					defaultMode: v.defaultMode
					name:        v.name
					if v.items != _|_ {
						items: v.items
					}
				}
			}
		},
	] | []

	secretVolumesList: *[
				if parameter.secret != _|_ for v in parameter.secret if v.mountPath != _|_ {
			{
				name: "secret-" + v.name
				secret: {
					defaultMode: v.defaultMode
					secretName:  v.name
					if v.items != _|_ {
						items: v.items
					}
				}
			}
		},
	] | []

	emptyDirVolumesList: *[
				if parameter.emptyDir != _|_ for v in parameter.emptyDir {
			{
				name: "emptydir-" + v.name
				emptyDir: {
					medium: v.medium
				}
			}
		},
	] | []

	pvcVolumeMountsList: *[
				if parameter.pvc != _|_ for v in parameter.pvc {
			if v.volumeMode == "Filesystem" {
				{
					name:      "pvc-" + v.name
					mountPath: v.mountPath
					if v.subPath != _|_ {
						subPath: v.subPath
					}
				}
			}
		},
	] | []

	configMapVolumeMountsList: *[
					if parameter.configMap != _|_ for v in parameter.configMap if v.mountPath != _|_ {
			{
				name:      "configmap-" + v.name
				mountPath: v.mountPath
				if v.subPath != _|_ {
					subPath: v.subPath
				}
			}
		},
	] | []

	configMapEnvMountsList: *[
				if parameter.configMap != _|_ for v in parameter.configMap if v.mountToEnv != _|_ {
			{
				name: v.mountToEnv.envName
				valueFrom: configMapKeyRef: {
					name: v.name
					key:  v.mountToEnv.configMapKey
				}
			}
		},
	] | []

	configMountToEnvsList: *[
				if parameter.configMap != _|_ for v in parameter.configMap if v.mountToEnvs != _|_ for k in v.mountToEnvs {
			{
				name: k.envName
				valueFrom: configMapKeyRef: {
					name: v.name
					key:  k.configMapKey
				}
			}
		},
	] | []

	secretVolumeMountsList: *[
				if parameter.secret != _|_ for v in parameter.secret if v.mountPath != _|_ {
			{
				name:      "secret-" + v.name
				mountPath: v.mountPath
				if v.subPath != _|_ {
					subPath: v.subPath
				}
			}
		},
	] | []

	secretEnvMountsList: *[
				if parameter.secret != _|_ if parameter.secret != _|_ for v in parameter.secret if v.mountToEnv != _|_ {
			{
				name: v.mountToEnv.envName
				valueFrom: secretKeyRef: {
					name: v.name
					key:  v.mountToEnv.secretKey
				}
			}
		},
	] | []

	secretMountToEnvsList: *[
				if parameter.secret != _|_ for v in parameter.secret if v.mountToEnvs != _|_ for k in v.mountToEnvs {
			{
				name: k.envName
				valueFrom: secretKeyRef: {
					name: v.name
					key:  k.secretKey
				}
			}
		},
	] | []

	emptyDirVolumeMountsList: *[
					if parameter.emptyDir != _|_ for v in parameter.emptyDir {
			{
				name:      "emptydir-" + v.name
				mountPath: v.mountPath
				if v.subPath != _|_ {
					subPath: v.subPath
				}
			}
		},
	] | []

	volumeDevicesList: *[
				if parameter.pvc != _|_ for v in parameter.pvc if v.volumeMode == "Block" {
			{
				name:       "pvc-" + v.name
				devicePath: v.mountPath
				if v.subPath != _|_ {
					subPath: v.subPath
				}
			}
		},
	] | []

	volumesList: pvcVolumesList + configMapVolumesList + secretVolumesList + emptyDirVolumesList
	deDupVolumesArray: [
		for val in [
			for i, vi in volumesList {
				for j, vj in volumesList if j < i && vi.name == vj.name {
					_ignore: true
				}
				vi
			},
		] if val._ignore == _|_ {
			val
		},
	]

	patch: spec: template: spec: {
		// +patchKey=name
		volumes: deDupVolumesArray

		containers: [{
			// +patchKey=name
			env: configMapEnvMountsList + secretEnvMountsList + configMountToEnvsList + secretMountToEnvsList
			// +patchKey=name
			volumeDevices: volumeDevicesList
			// +patchKey=name
			volumeMounts: pvcVolumeMountsList + configMapVolumeMountsList + secretVolumeMountsList + emptyDirVolumeMountsList
		}, ...]

	}

	outputs: {
		if parameter.pvc != _|_ for v in parameter.pvc {
			if v.mountOnly == false {
				"pvc-\(v.name)": {
					apiVersion: "v1"
					kind:       "PersistentVolumeClaim"
					metadata: {
						name: v.name
					}
					spec: {
						accessModes: v.accessModes
						volumeMode:  v.volumeMode
						if v.volumeName != _|_ {
							volumeName: v.volumeName
						}
						if v.storageClassName != _|_ {
							storageClassName: v.storageClassName
						}

						if v.resources.requests.storage == _|_ {
							resources: requests: storage: "8Gi"
						}
						if v.resources.requests.storage != _|_ {
							resources: requests: storage: v.resources.requests.storage
						}
						if v.resources.limits.storage != _|_ {
							resources: limits: storage: v.resources.limits.storage
						}
						if v.dataSourceRef != _|_ {
							dataSourceRef: v.dataSourceRef
						}
						if v.dataSource != _|_ {
							dataSource: v.dataSource
						}
						if v.selector != _|_ {
							dataSource: v.selector
						}
					}
				}
			}
		}

		if parameter.configMap != _|_ for v in parameter.configMap {
			if v.mountOnly == false {
				"configmap-\(v.name)": {
					apiVersion: "v1"
					kind:       "ConfigMap"
					metadata: name: v.name
					if v.data != _|_ {
						data: v.data
					}
				}
			}
		}

		if parameter.secret != _|_ for v in parameter.secret {
			if v.mountOnly == false {
				"secret-\(v.name)": {
					apiVersion: "v1"
					kind:       "Secret"
					metadata: name: v.name
					if v.data != _|_ {
						data: v.data
					}
					if v.stringData != _|_ {
						stringData: v.stringData
					}
				}
			}
		}

	}

	parameter: {
		// +usage=Declare pvc type storage
		pvc?: [...{
			name:              string
			mountOnly:         *false | bool
			mountPath:         string
			subPath?:          string
			volumeMode:        *"Filesystem" | string
			volumeName?:       string
			accessModes:       *["ReadWriteOnce"] | [...string]
			storageClassName?: string
			resources?: {
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
		}]

		// +usage=Declare config map type storage
		configMap?: [...{
			name:      string
			mountOnly: *false | bool
			mountToEnv?: {
				envName:      string
				configMapKey: string
			}
			mountToEnvs?: [...{
				envName:      string
				configMapKey: string
			}]
			mountPath?:  string
			subPath?:    string
			defaultMode: *420 | int
			readOnly:    *false | bool
			data?: {...}
			items?: [...{
				key:  string
				path: string
				mode: *511 | int
			}]
		}]

		// +usage=Declare secret type storage
		secret?: [...{
			name:      string
			mountOnly: *false | bool
			mountToEnv?: {
				envName:   string
				secretKey: string
			}
			mountToEnvs?: [...{
				envName:   string
				secretKey: string
			}]
			mountPath?:  string
			subPath?:    string
			defaultMode: *420 | int
			readOnly:    *false | bool
			stringData?: {...}
			data?: {...}
			items?: [...{
				key:  string
				path: string
				mode: *511 | int
			}]
		}]

		// +usage=Declare empty dir type storage
		emptyDir?: [...{
			name:      string
			mountPath: string
			subPath?:  string
			medium:    *"" | "Memory"
		}]
	}

}
