import "vela/test"

_web: {
	definition: "storage"
	context: name: "web"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [
			{name: "web", image: "shop:1.0"},
			{name: "log", image: "fluentbit:3"},
		]
	}
}

"a PVC is created, mounted into the first container": test.#TraitRender & _web & {
	parameter: pvc: [{name: "data", mountPath: "/data"}]
	expect: {
		output: spec: template: spec: {
			volumes: [{name: "pvc-data", persistentVolumeClaim: claimName: "data"}]
			containers: [
				{name: "web", volumeMounts: [{name: "pvc-data", mountPath: "/data"}], volumeDevices: []},
				{name: "log", volumeMounts?: _|_},
			]
		}
		outputs: {
			"pvc-data": {
				apiVersion: "v1"
				kind:       "PersistentVolumeClaim"
				metadata: name: "data"
				spec: {
					accessModes: ["ReadWriteOnce"]
					volumeMode: "Filesystem"
					resources: {requests: storage: "8Gi"} @exact()
				} @exact()
			}
		} @exact()
	}
}

"a PVC's class, size, volume and data source pass through": test.#TraitRender & _web & {
	parameter: pvc: [{
		name:      "data"
		mountPath: "/data"
		subPath:   "shop"
		accessModes: ["ReadWriteMany"]
		storageClassName: "fast"
		volumeName:       "pv-0001"
		resources: {requests: storage: "20Gi", limits: storage: "50Gi"}
		dataSource: {name: "data-snap", kind: "VolumeSnapshot", apiGroup: "snapshot.storage.k8s.io"}
	}]
	expect: {
		output: spec: template: spec: containers: [{volumeMounts: [{name: "pvc-data", mountPath: "/data", subPath: "shop"}]}, _]
		outputs: "pvc-data": spec: {
			accessModes: ["ReadWriteMany"]
			storageClassName: "fast"
			volumeName:       "pv-0001"
			resources: {requests: storage: "20Gi", limits: storage: "50Gi"}
			dataSource: {name: "data-snap", kind: "VolumeSnapshot", apiGroup: "snapshot.storage.k8s.io"}
		}
	}
}

"a selector narrows the volumes the PVC can bind": test.#TraitRender & _web & {
	parameter: pvc: [{name: "data", mountPath: "/data", selector: matchLabels: tier: "gold"}]
	expect: outputs: "pvc-data": spec: {
		selector: matchLabels: tier: "gold"
		dataSource?: _|_
	}
}

"a selector's match expressions are a list of requirements": test.#TraitRender & _web & {
	parameter: pvc: [{name: "data", mountPath: "/data", selector: matchExpressions: [
		{key: "tier", operator: "In", values: ["gold", "silver"]},
		{key: "zone", operator: "Exists"},
	]}]
	expect: outputs: "pvc-data": spec: selector: matchExpressions: [
		{key: "tier", operator: "In", values: ["gold", "silver"]},
		{key: "zone", operator: "Exists"},
	]
}

"a block PVC is attached as a device": test.#TraitRender & _web & {
	parameter: pvc: [{name: "raw", mountPath: "/dev/xvda", volumeMode: "Block"}]
	expect: {
		output: spec: template: spec: containers: [{volumeDevices: [{name: "pvc-raw", devicePath: "/dev/xvda"}]}, _]
		outputs: "pvc-raw": spec: volumeMode: "Block"
	}
}

"a block PVC is not mounted": test.#TraitRender & _web & {
	parameter: pvc: [{name: "raw", mountPath: "/dev/xvda", volumeMode: "Block"}]
	expect: output: spec: template: spec: containers: [{volumeMounts: []}, _]
}

"mountOnly mounts an existing PVC without creating it": test.#TraitRender & _web & {
	parameter: pvc: [{name: "data", mountPath: "/data", mountOnly: true}]
	expect: {
		output: spec: template: spec: {
			volumes: [{name: "pvc-data", persistentVolumeClaim: claimName: "data"}]
			containers: [{name: "web", volumeMounts: [{name: "pvc-data", mountPath: "/data"}]}, {name: "log", volumeMounts?: _|_}]
		}
		outputs: {} @exact()
	}
}

"one PVC at two paths is a single volume with two mounts": test.#TraitRender & _web & {
	parameter: pvc: [
		{name: "data", mountPath: "/data", mountOnly: true},
		{name: "data", mountPath: "/backup", mountOnly: true},
	]
	expect: output: spec: template: spec: {
		volumes: [{name: "pvc-data"}]
		containers: [{volumeMounts: [
			{name: "pvc-data", mountPath: "/data"},
			{name: "pvc-data", mountPath: "/backup"},
		]}, _]
	}
}

"a ConfigMap is created and mounted with its data": test.#TraitRender & _web & {
	parameter: configMap: [{name: "shop-config", mountPath: "/etc/shop", data: "shop.yaml": "mode: prod"}]
	expect: {
		output: spec: template: spec: {
			volumes: [{name: "configmap-shop-config", configMap: {name: "shop-config", defaultMode: 420, items?: _|_}}]
			containers: [{volumeMounts: [{name: "configmap-shop-config", mountPath: "/etc/shop"}]}, _]
		}
		outputs: {
			"configmap-shop-config": {
				apiVersion: "v1"
				kind:       "ConfigMap"
				metadata: name:    "shop-config"
				data: "shop.yaml": "mode: prod"
			}
		} @exact()
	}
}

"a ConfigMap's items project chosen keys": test.#TraitRender & _web & {
	parameter: configMap: [{
		name:        "shop-config"
		mountPath:   "/etc/shop"
		mountOnly:   true
		defaultMode: 256
		items: [{key: "shop.yaml", path: "config.yaml"}]
	}]
	expect: output: spec: template: spec: volumes: [{configMap: {
		defaultMode: 256
		items: [{key: "shop.yaml", path: "config.yaml", mode: 511}]
	}}]
}

"a ConfigMap with no mountPath feeds environment variables only": test.#TraitRender & _web & {
	parameter: configMap: [{
		name:      "shop-config"
		mountOnly: true
		mountToEnv: {envName: "MODE", configMapKey: "mode"}
		mountToEnvs: [{envName: "REGION", configMapKey: "region"}]
	}]
	expect: output: spec: template: spec: {
		volumes: []
		containers: [{
			volumeMounts: []
			env: [
				{name: "MODE", valueFrom: configMapKeyRef: {name: "shop-config", key: "mode"}},
				{name: "REGION", valueFrom: configMapKeyRef: {name: "shop-config", key: "region"}},
			]
		}, _]
	}
}

"a Secret is created and mounted, and can feed environment variables": test.#TraitRender & _web & {
	parameter: secret: [{
		name:      "db-conn"
		mountPath: "/etc/db"
		stringData: password: "hunter2"
		mountToEnv: {envName: "DB_PASSWORD", secretKey: "password"}
	}]
	expect: {
		output: spec: template: spec: {
			volumes: [{name: "secret-db-conn", secret: {secretName: "db-conn", defaultMode: 420}}]
			containers: [{
				volumeMounts: [{name: "secret-db-conn", mountPath: "/etc/db"}]
				env: [{name: "DB_PASSWORD", valueFrom: secretKeyRef: {name: "db-conn", key: "password"}}]
			}, _]
		}
		outputs: {
			"secret-db-conn": {
				apiVersion: "v1"
				kind:       "Secret"
				metadata: name:       "db-conn"
				stringData: password: "hunter2"
				data?: _|_
			}
		} @exact()
	}
}

"an emptyDir is mounted, on disk by default": test.#TraitRender & _web & {
	parameter: emptyDir: [{name: "cache", mountPath: "/cache"}]
	expect: {
		output: spec: template: spec: {
			volumes: [{name: "emptydir-cache", emptyDir: {medium: ""} @exact()}]
			containers: [{volumeMounts: [{name: "emptydir-cache", mountPath: "/cache"}]}, _]
		}
		outputs: {} @exact()
	}
}

"an emptyDir can live in memory": test.#TraitRender & _web & {
	parameter: emptyDir: [{name: "cache", mountPath: "/cache", medium: "Memory"}]
	expect: output: spec: template: spec: volumes: [{emptyDir: medium: "Memory"}]
}

"a hostPath is mounted, expecting a directory by default": test.#TraitRender & _web & {
	parameter: hostPath: [{name: "logs", path: "/var/log", mountPath: "/host/log"}]
	expect: {
		output: spec: template: spec: {
			volumes: [{name: "hostpath-logs", hostPath: {path: "/var/log", type: "Directory"}}]
			containers: [{volumeMounts: [{name: "hostpath-logs", mountPath: "/host/log"}]}, _]
		}
		outputs: {} @exact()
	}
}

"volumes of every kind sit side by side": test.#TraitRender & _web & {
	parameter: {
		pvc: [{name: "data", mountPath: "/data", mountOnly: true}]
		configMap: [{name: "shop-config", mountPath: "/etc/shop", mountOnly: true}]
		secret: [{name: "db-conn", mountPath: "/etc/db", mountOnly: true}]
		emptyDir: [{name: "cache", mountPath: "/cache"}]
		hostPath: [{name: "logs", path: "/var/log", mountPath: "/host/log"}]
	}
	expect: output: spec: template: spec: volumes: [
		{name: "pvc-data"},
		{name: "configmap-shop-config"},
		{name: "secret-db-conn"},
		{name: "emptydir-cache"},
		{name: "hostpath-logs"},
	]
}

"a PVC size must carry a unit": test.#TraitRender & _web & {
	parameter: pvc: [{name: "data", mountPath: "/data", resources: requests: storage: "20"}]
	expect: error: {
		parameter: [=~"storage"] @contains()
	}
}

"an unknown hostPath type is rejected": test.#TraitRender & _web & {
	parameter: hostPath: [{name: "logs", path: "/var/log", mountPath: "/host/log", type: "Pipe"}]
	expect: error: {
		parameter: [=~"type"] @contains()
	}
}

"a PVC can be filled from a data source reference": test.#TraitRender & _web & {
	parameter: pvc: [{
		name:      "data"
		mountPath: "/data"
		dataSourceRef: {name: "seed", kind: "VolumePopulator", apiGroup: "populator.example.com"}
	}]
	expect: outputs: "pvc-data": spec: {
		dataSourceRef: {name: "seed", kind: "VolumePopulator", apiGroup: "populator.example.com"} @exact()
		dataSource?: _|_
	}
}

"a ConfigMap, Secret and emptyDir can each mount at a subPath": test.#TraitRender & _web & {
	parameter: {
		configMap: [{name: "shop-config", mountPath: "/etc/shop/shop.yaml", subPath: "shop.yaml", mountOnly: true}]
		secret: [{name: "db-conn", mountPath: "/etc/db/password", subPath: "password", mountOnly: true}]
		emptyDir: [{name: "cache", mountPath: "/cache", subPath: "web"}]
	}
	expect: output: spec: template: spec: containers: [{volumeMounts: [
		{name: "configmap-shop-config", mountPath: "/etc/shop/shop.yaml", subPath: "shop.yaml"},
		{name: "secret-db-conn", mountPath: "/etc/db/password", subPath: "password"},
		{name: "emptydir-cache", mountPath: "/cache", subPath: "web"},
	]}, _]
}

"a Secret's data, mode and items, and a list of environment variables": test.#TraitRender & _web & {
	parameter: secret: [{
		name:        "db-conn"
		mountPath:   "/etc/db"
		defaultMode: 256
		data: password: "aHVudGVyMg=="
		items: [{key: "password", path: "db-password"}]
		mountToEnvs: [
			{envName: "DB_PASSWORD", secretKey: "password"},
			{envName: "DB_USER", secretKey: "user"},
		]
	}]
	expect: {
		output: spec: template: spec: {
			volumes: [{name: "secret-db-conn", secret: {
				secretName:  "db-conn"
				defaultMode: 256
				items: [{key: "password", path: "db-password", mode: 511}]
			}}]
			containers: [{env: [
				{name: "DB_PASSWORD", valueFrom: secretKeyRef: {name: "db-conn", key: "password"}},
				{name: "DB_USER", valueFrom: secretKeyRef: {name: "db-conn", key: "user"}},
			]}, _]
		}
		outputs: "secret-db-conn": {data: password: "aHVudGVyMg==", stringData?: _|_}
	}
}

"a read-only ConfigMap or Secret is mounted read-only": test.#TraitRender & _web & {
	parameter: {
		configMap: [{name: "shop-config", mountPath: "/etc/shop", readOnly: true, mountOnly: true}]
		secret: [{name: "db-conn", mountPath: "/etc/db", readOnly: true, mountOnly: true}]
	}
	expect: output: spec: template: spec: containers: [{volumeMounts: [
		{name: "configmap-shop-config", readOnly: true},
		{name: "secret-db-conn", readOnly: true},
	]}, _]
}
