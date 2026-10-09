import "vela/test"

_web: {
	definition: "nocalhost"
	context: {name: "web", appName: "shop", namespace: "dev"}
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [{name: "web", image: "shop:1.0"}]
	}
	parameter: {
		port: 8080
		...
	}
}

"exposes the port through a Service": test.#TraitRender & _web & {
	parameter: image: "go"
	expect: {
		outputs: {
			nocalhostService: {
				apiVersion: "v1"
				kind:       "Service"
				metadata: name: "web"
				spec: {
					selector: "app.oam.dev/component": "web"
					type: "ClusterIP"
					ports: [{port: 8080, targetPort: 8080}]
				}
			}
		} @exact()
	}
}

"annotates the workload with its application and dev configuration": test.#TraitRender & _web & {
	parameter: image: "go"
	expect: output: metadata: annotations: {
		"dev.nocalhost/application-name":      "shop"
		"dev.nocalhost/application-namespace": "dev"
		"dev.nocalhost":                       =~"^\\{\"name\":\"web\",\"serviceType\":\"deployment\",\"containers\":\\[\\{\"name\":\"web\",\"dev\":\\{"
	}
}

"a language picks the matching dev image": test.#TraitRender & _web & {
	parameter: image: "python"
	expect: output: metadata: annotations: "dev.nocalhost": =~"\"image\":\"nocalhost-docker.pkg.coding.net/nocalhost/dev-images/python:latest\""
}

"any other image is used as given": test.#TraitRender & _web & {
	parameter: image: "registry.example.com/shop-dev:1.0"
	expect: output: metadata: annotations: "dev.nocalhost": =~"\"image\":\"registry.example.com/shop-dev:1.0\""
}

"the service port is forwarded by default": test.#TraitRender & _web & {
	parameter: image: "go"
	expect: output: metadata: annotations: "dev.nocalhost": =~"\"portForward\":\\[\"8080:8080\"\\]"
}

"explicit port forwards replace the default": test.#TraitRender & _web & {
	parameter: {image: "go", portForward: ["9000:8080"]}
	expect: output: metadata: annotations: "dev.nocalhost": =~"\"portForward\":\\[\"9000:8080\"\\]"
}

"dev settings are carried in the configuration": test.#TraitRender & _web & {
	parameter: {
		image:        "go"
		gitUrl:       "https://example.com/shop.git"
		storageClass: "fast"
		env: [{name: "MODE", value: "dev"}]
		persistentVolumeDirs: [{path: "/cache", capacity: "1Gi"}]
	}
	expect: output: metadata: annotations: "dev.nocalhost": =~"\"gitUrl\":\"https://example.com/shop.git\"" & =~"\"storageClass\":\"fast\"" & =~"\"env\":\\[\\{\"name\":\"MODE\",\"value\":\"dev\"\\}\\]" & =~"\"persistentVolumeDirs\":\\[\\{\"path\":\"/cache\",\"capacity\":\"1Gi\"\\}\\]"
}

"a port is required": test.#TraitRender & {
	definition: "nocalhost"
	context:    _web.context
	workload:   _web.workload
	parameter: image: "go"
	expect: error:    =~"portForward"
}
