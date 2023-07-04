configmap: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Add configmaps on K8s pod for your workload which follows the pod spec in path 'spec.template'."
	attributes: {
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch"]
		podDisruptive: true
	}
}

template: {
	volumesList: [
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
	]

	volumeMountsList: [
		if parameter.configMap != _|_ for v in parameter.configMap if v.mountPath != _|_ {
			{
				name:      "configmap-" + v.name
				mountPath: v.mountPath
				if v.subPath != _|_ {
					subPath: v.subPath
				}
			}
		},
	]

	envList: [
		if parameter.configMap != _|_ for v in parameter.configMap if v.mountToEnv != _|_ {
			{
				name: v.mountToEnv.envName
				valueFrom: configMapKeyRef: {
					name: v.name
					key:  v.mountToEnv.configMapKey
				}
			}
		},
		if parameter.configMap != _|_ for v in parameter.configMap if v.mountToEnvs != _|_ for k in v.mountToEnvs {
			{
				name: k.envName
				valueFrom: configMapKeyRef: {
					name: v.name
					key:  k.configMapKey
				}
			}
		},
	]

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
			env: envList
			// +patchKey=name
			volumeMounts: volumeMountsList
		}, ...]

	}

	outputs: {
		for v in parameter.configMap {
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

	parameter: {
		configMap?: [...{
			name: string
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
	}
}
