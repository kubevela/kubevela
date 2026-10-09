import "vela/test"

_app: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	metadata: {name: "upstream", namespace: "deps"}
	spec: components: [{name: "web", type: "webservice", properties: image: "nginx"}]
}

_dependsOn: {
	definition: "depends-on-app"
	parameter: {name: "upstream", namespace: "deps"}
}

"succeeds once the Application is running": test.#WorkflowStepExec & _dependsOn & {
	resources: [_app & {status: status: "running"}]
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Read": [{$params: value: {kind: "Application", metadata: {name: "upstream", namespace: "deps"}}}]
	}
}

"waits while the Application is not running": test.#WorkflowStepExec & _dependsOn & {
	resources: [_app & {status: status: "rendering"}]
	expect: phase: "running"
}

"applies the Application from a ConfigMap of the same name when it does not exist": test.#WorkflowStepExec & _dependsOn & {
	resources: [{
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {name: "upstream", namespace: "deps"}
		data: application: """
			apiVersion: core.oam.dev/v1beta1
			kind: Application
			metadata:
			  name: upstream
			  namespace: deps
			spec:
			  components:
			  - name: web
			    type: webservice
			    properties:
			      image: nginx
			"""
	}]
	expect: {
		phase: "running"
		calls: "vela/kube": "#Apply": [{$params: value: {kind: "Application", metadata: name: "upstream"}}]
		resources: [_app]
	}
}

"waits while the Application has no status yet": test.#WorkflowStepExec & _dependsOn & {
	resources: [_app]
	expect: phase: "running"
}

"fails when neither the Application nor its ConfigMap exists": test.#WorkflowStepExec & _dependsOn & {
	expect: phase: "failed"
}

"the namespace is required": test.#WorkflowStepExec & {
	definition: "depends-on-app"
	parameter: name: "upstream"
	expect: {
		phase:   "failed"
		message: =~"parameter.namespace"
	}
}
