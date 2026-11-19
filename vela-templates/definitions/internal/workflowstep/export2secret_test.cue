import (
	"vela/test"
	"vela/kube"
)

tidy: kube.#Delete & {$params: value: {apiVersion: "v1", kind: "Secret", metadata: {name: "creds", namespace: "export2secret-target"}}} @afterEach()

"exports the data to an Opaque Secret in the step's namespace": test.#WorkflowStepExec & {
	definition: "export2secret"
	parameter: {secretName: "creds", data: {user: "admin", password: "s3cret"}}
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Apply": [{$params: {
			cluster: ""
			value: {kind: "Secret", metadata: name: "creds", stringData: {user: "admin", password: "s3cret"}, type?: _|_}
		}}]
		resources: [{
			apiVersion: "v1"
			kind:       "Secret"
			metadata: name: "creds"
			type: "Opaque"
			data: {user: "YWRtaW4=", password: "czNjcmV0"} @exact()
		}]
	}
}

"exports into the given namespace": test.#WorkflowStepExec & {
	definition: "export2secret"
	parameter: {secretName: "creds", namespace: "export2secret-target", data: user: "admin"}
	resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "marker", namespace: "export2secret-target"}}]
	expect: resources: [{apiVersion: "v1", kind: "Secret", metadata: {name: "creds", namespace: "export2secret-target"}, data: user: "YWRtaW4="}]
}

"sets the given type": test.#WorkflowStepExec & {
	definition: "export2secret"
	parameter: {secretName: "creds", type: "kubernetes.io/basic-auth", data: {username: "admin", password: "s3cret"}}
	expect: resources: [{apiVersion: "v1", kind: "Secret", metadata: name: "creds", type: "kubernetes.io/basic-auth"}]
}

_registry: {
	definition: "export2secret"
	parameter: {
		secretName: "pull"
		kind:       "docker-registry"
		data: {}
		dockerRegistry: {username: "bot", password: "s3cret"}
	}
}

"builds a docker config for Docker Hub by default": test.#WorkflowStepExec & _registry & {
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Apply": [{$params: value: {
			type: "kubernetes.io/dockerconfigjson"
			stringData: ".dockerconfigjson": "{\"auths\":{\"https://index.docker.io/v1/\":{\"username\":\"bot\",\"password\":\"s3cret\",\"auth\":\"Ym90OnMzY3JldA==\"}}}"
		}}]
		resources: [{apiVersion: "v1", kind: "Secret", metadata: name: "pull", type: "kubernetes.io/dockerconfigjson"}]
	}
}

"builds the docker config for the given registry server": test.#WorkflowStepExec & _registry & {
	parameter: dockerRegistry: server: "registry.example.com"
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Apply": [{$params: value: stringData: ".dockerconfigjson": "{\"auths\":{\"registry.example.com\":{\"username\":\"bot\",\"password\":\"s3cret\",\"auth\":\"Ym90OnMzY3JldA==\"}}}"}]
	}
}

"a docker-registry Secret without registry details exports the data as it is": test.#WorkflowStepExec & {
	definition: "export2secret"
	parameter: {
		secretName: "pull"
		kind:       "docker-registry"
		data: ".dockerconfigjson": "{\"auths\":{}}"
	}
	expect: {
		phase: "succeeded"
		resources: [{
			apiVersion: "v1"
			kind:       "Secret"
			metadata: name: "pull"
			type: "kubernetes.io/dockerconfigjson"
			data: ".dockerconfigjson": "eyJhdXRocyI6e319"
		}]
	}
}

"passes the cluster through": test.#WorkflowStepExec & {
	definition: "export2secret"
	parameter: {secretName: "creds", data: user: "admin", cluster: "eu-1"}
	mocks: "vela/kube": "#Apply": $returns: value: {}
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Apply": [{$params: cluster: "eu-1"}]
	}
}

"the Secret name is required": test.#WorkflowStepExec & {
	definition: "export2secret"
	parameter: data: user: "admin"
	expect: {
		phase:   "failed"
		message: =~"parameter.secretName"
	}
}
