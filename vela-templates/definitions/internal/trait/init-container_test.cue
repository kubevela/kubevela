import "vela/test"

_web: {
	definition: "init-container"
	context: {name: "web", appName: "shop"}
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [
			{name: "web", image: "shop:1.0"},
			{name: "proxy", image: "envoy:1.30"},
		]
	}
	parameter: {
		name:          "fetch"
		image:         "busybox:1.36"
		appMountPath:  "/srv/data"
		initMountPath: "/work"
		...
	}
}

"adds an init container sharing an emptyDir with the component's container": test.#TraitRender & _web & {
	expect: output: spec: template: spec: {
		initContainers: [{
			name:            "fetch"
			image:           "busybox:1.36"
			imagePullPolicy: "IfNotPresent"
			command?:        _|_
			args?:           _|_
			env?:            _|_
			volumeMounts: [{name: "workdir", mountPath: "/work"}]
		}]
		containers: [
			{name: "web", image: "shop:1.0", volumeMounts: [{name: "workdir", mountPath: "/srv/data"}]},
			{name: "proxy", volumeMounts?: _|_},
		]
		volumes: [{name: "workdir", emptyDir: {}}]
	}
}

"command, args and env are passed to the init container": test.#TraitRender & _web & {
	parameter: {
		cmd: ["sh", "-c"]
		args: ["wget -O /work/index.html example.com"]
		env: [{name: "TOKEN", valueFrom: secretKeyRef: {name: "creds", key: "token"}}]
		imagePullPolicy: "Always"
	}
	expect: output: spec: template: spec: initContainers: [{
		imagePullPolicy: "Always"
		command: ["sh", "-c"]
		args: ["wget -O /work/index.html example.com"]
		env: [{name: "TOKEN", valueFrom: secretKeyRef: {name: "creds", key: "token"}}]
	}]
}

"the shared volume can be renamed": test.#TraitRender & _web & {
	parameter: mountName: "shared"
	expect: output: spec: template: spec: {
		initContainers: [{volumeMounts: [{name: "shared"}]}]
		containers: [{volumeMounts: [{name: "shared"}]}, {}]
		volumes: [{name: "shared"}]
	}
}

"extra volume mounts follow the shared one on the init container": test.#TraitRender & _web & {
	parameter: extraVolumeMounts: [{name: "config", mountPath: "/etc/fetch"}]
	expect: output: spec: template: spec: initContainers: [{volumeMounts: [
		{name: "workdir", mountPath: "/work"},
		{name: "config", mountPath: "/etc/fetch"},
	]}]
}

"existing init containers are kept": test.#TraitRender & _web & {
	workload: spec: template: spec: initContainers: [{name: "migrate", image: "shop-migrate:1.0"}]
	expect: output: spec: template: spec: initContainers: [{name: "migrate"}, {name: "fetch"}]
}

"existing volumes and mounts are kept beside the shared ones": test.#TraitRender & _web & {
	workload: spec: template: spec: {
		containers: [{name: "web", image: "shop:1.0", volumeMounts: [{name: "config", mountPath: "/etc/shop"}]}, {name: "proxy"}]
		volumes: [{name: "config", configMap: name: "shop"}]
	}
	expect: output: spec: template: spec: {
		containers: [{volumeMounts: [{name: "config", mountPath: "/etc/shop"}, {name: "workdir", mountPath: "/srv/data"}]}, {}]
		volumes: [{name: "config", configMap: name: "shop", emptyDir?: _|_}, {name: "workdir", emptyDir: {}}]
	}
}

"the mount paths are required": test.#TraitRender & {
	definition: "init-container"
	context: name: "web"
	workload: _web.workload
	parameter: {name: "fetch", image: "busybox:1.36"}
	expect: error: {
		template: [=~"containers.0.volumeMounts.0.mountPath", =~"initContainers.0.volumeMounts.0.mountPath"] @contains()
	}
}

"an unknown pull policy is rejected": test.#TraitRender & _web & {
	parameter: imagePullPolicy: "Sometimes"
	expect: error: {
		parameter: [=~"imagePullPolicy"] @contains()
	}
}

"a plain env value is passed to the init container": test.#TraitRender & _web & {
	parameter: env: [{name: "TARGET", value: "/work"}]
	expect: output: spec: template: spec: initContainers: [{env: [{name: "TARGET", value: "/work"}]}]
}
