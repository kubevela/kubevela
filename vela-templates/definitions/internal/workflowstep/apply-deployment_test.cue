import "vela/test"

// _ready is the step's Deployment as its controller would leave it, with _n
// replicas ready.
_ready: {
	_n: *1 | int
	resources: [{
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "test-step"
		spec: {
			replicas: _n
			selector: matchLabels: "workflow.oam.dev/step-name": "test-app-test-step"
			template: {
				metadata: labels: "workflow.oam.dev/step-name": "test-app-test-step"
				spec: containers: [{name: "test-step", image: "busybox"}]
			}
		}
		status: {replicas: _n, readyReplicas: _n}
	}]
}

"applies a deployment named after the step": test.#WorkflowStepExec & {
	definition: "apply-deployment"
	parameter: image: "nginx:1.25"
	expect: {
		calls: "vela/kube": "#Apply": [{$params: {cluster: "", value: metadata: name: "test-step"}}]
		resources: [{
			apiVersion: "apps/v1"
			kind:       "Deployment"
			metadata: name: "test-step"
			spec: {
				replicas: 1
				selector: matchLabels: "workflow.oam.dev/step-name": "test-app-test-step"
				template: {
					metadata: labels: "workflow.oam.dev/step-name": "test-app-test-step"
					spec: containers: [{name: "test-step", image: "nginx:1.25", command?: _|_}]
				}
			}
		}]
	}
}

// A new Deployment reports no readyReplicas until its controller sets them.
"waits while the deployment reports no ready replicas": test.#WorkflowStepExec & {
	definition: "apply-deployment"
	parameter: image: "nginx:1.25"
	expect: phase:    "running"
}

"succeeds once the replicas are ready": test.#WorkflowStepExec & _ready & {
	definition: "apply-deployment"
	parameter: image: "nginx:1.25"
	expect: phase:    "succeeded"
}

"keeps waiting while fewer replicas are ready than asked for": test.#WorkflowStepExec & _ready & {
	definition: "apply-deployment"
	parameter: {image: "nginx:1.25", replicas: 3}
	expect: {
		phase: "running"
		resources: [{apiVersion: "apps/v1", kind: "Deployment", metadata: name: "test-step", spec: replicas: 3}]
	}
}

"succeeds once every one of several replicas is ready": test.#WorkflowStepExec & _ready & {
	_n:         3
	definition: "apply-deployment"
	parameter: {image: "nginx:1.25", replicas: 3}
	expect: phase: "succeeded"
}

// A Deployment scaled to zero never reports readyReplicas.
"succeeds at once with zero replicas": test.#WorkflowStepExec & {
	definition: "apply-deployment"
	parameter: {image: "nginx:1.25", replicas: 0}
	expect: phase: "succeeded"
}

"runs the given command": test.#WorkflowStepExec & {
	definition: "apply-deployment"
	context: stepName: "migrate"
	parameter: {image: "busybox", cmd: ["sh", "-c", "echo done"]}
	expect: resources: [{
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "migrate"
		spec: template: spec: containers: [{name: "migrate", command: ["sh", "-c", "echo done"]}]
	}]
}

"passes the cluster through": test.#WorkflowStepExec & {
	definition: "apply-deployment"
	parameter: {image: "nginx", cluster: "eu-1"}
	mocks: "vela/kube": "#Apply": $returns: value: status: readyReplicas: 1
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Apply": [{$params: cluster: "eu-1"}]
	}
}

"an image is required": test.#WorkflowStepExec & {
	definition: "apply-deployment"
	expect: {
		phase:   "failed"
		message: =~"image: cannot convert non-concrete value"
	}
}
