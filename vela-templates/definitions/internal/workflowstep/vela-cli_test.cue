import (
	"vela/kube"
	"vela/test"
)

// The step's Job goes to vela-system by default, which the test cluster lacks.
velaSystem: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "Namespace", metadata: name: "vela-system"}} @before()

// Each case names its own Application, and so its own Job: the API server
// orphans a deleted batch/v1 Job behind a finalizer only the garbage
// collector removes, so a Job cannot be tidied away between cases.
_cli: {
	_app:       string
	definition: "vela-cli"
	context: {namespace: "vela-cli", appName: _app}
	parameter: command: ["vela", "addon", "enable", "fluxcd"]
}

// _job is the Job the step applies with the default parameters.
_job: {
	_app:       string
	apiVersion: "batch/v1"
	kind:       "Job"
	metadata: {name: "\(_app)-test-step-test-step-id", namespace: "vela-system"}
	spec: {
		backoffLimit: 3
		template: {
			metadata: labels: "workflow.oam.dev/step-name": "\(_app)-test-step"
			spec: {
				containers: [{
					name:  "\(_app)-test-step-test-step-id-job"
					image: "oamdev/vela-cli:v1.6.4"
					command: ["vela", "addon", "enable", "fluxcd"]
					// The API server drops the step's empty lists.
					volumeMounts?: _|_
				}]
				restartPolicy:  "Never"
				serviceAccount: "kubevela-vela-core"
				volumes?:       _|_
			}
		}
	}
}

"runs the command in a Job and waits for it": test.#WorkflowStepExec & _cli & {
	_app: "enable"
	expect: {
		phase: "running"
		calls: {
			"vela/kube": "#Apply": [{$params: value: metadata: {name: "enable-test-step-test-step-id", namespace: "vela-system"}}]
			"vela/util": "#Log": [{$params: source: resources: [{labelSelector: "workflow.oam.dev/step-name": "enable-test-step"}]}]
		}
		resources: [_job & {_app: "enable"}]
	}
}

"runs in the step's namespace with another service account": test.#WorkflowStepExec & _cli & {
	_app: "own-sa"
	parameter: {serviceAccountName: "deployer", image: "oamdev/vela-cli:v1.9.0"}
	expect: {
		phase: "running"
		resources: [{
			apiVersion: "batch/v1"
			kind:       "Job"
			metadata: {name: "own-sa-test-step-test-step-id", namespace: "vela-cli"}
			spec: template: spec: {
				serviceAccountName: "deployer"
				containers: [{image: "oamdev/vela-cli:v1.9.0"}]
			}
		}]
	}
}

"mounts Secrets, once per name": test.#WorkflowStepExec & _cli & {
	_app: "secrets"
	parameter: storage: secret: [
		{name: "kubeconfig", mountPath: "/root/.kube", secretName: "hub-kubeconfig", items: [{key: "config", path: "config"}]},
		{name: "kubeconfig", mountPath: "/etc/kubeconfig", subPath: "config", secretName: "hub-kubeconfig", defaultMode: 256},
	]
	expect: resources: [{
		apiVersion: "batch/v1"
		kind:       "Job"
		metadata: {name: "secrets-test-step-test-step-id", namespace: "vela-system"}
		spec: template: spec: {
			containers: [{volumeMounts: [
				{name: "secret-kubeconfig", mountPath: "/root/.kube"},
				{name: "secret-kubeconfig", mountPath: "/etc/kubeconfig", subPath: "config"},
			]}]
			volumes: [{
				name: "secret-kubeconfig"
				secret: {secretName: "hub-kubeconfig", defaultMode: 420, items: [{key: "config", path: "config", mode: 511}]}
			}]
		}
	}]
}

// The template writes path beside the volume's name rather than under
// hostPath, so the API server, finding no volume source, makes it an emptyDir.
"mounts a host path": test.#WorkflowStepExec & _cli & {
	_app: "host"
	parameter: storage: hostPath: [{name: "docker", path: "/var/run/docker.sock", mountPath: "/var/run/docker.sock", type: "Socket"}]
	expect: resources: [{
		apiVersion: "batch/v1"
		kind:       "Job"
		metadata: {name: "host-test-step-test-step-id", namespace: "vela-system"}
		spec: template: spec: {
			containers: [{volumeMounts: [{name: "hostpath-docker", mountPath: "/var/run/docker.sock"}]}]
			volumes: [{name: "hostpath-docker", hostPath: {path: "/var/run/docker.sock", type: "Socket"}}]
		}
	}]
} @pending(vela-cli emits a hostPath volume as name and path with no hostPath source, so the API server stores it as an emptyDir)

"finishes once the Job has succeeded": test.#WorkflowStepExec & _cli & {
	_app: "done"
	resources: [_job & {
		_app: "done"
		status: {
			succeeded:      1
			startTime:      "2026-01-01T00:00:00Z"
			completionTime: "2026-01-01T00:01:00Z"
			conditions: [
				{type: "SuccessCriteriaMet", status: "True"},
				{type: "Complete", status: "True"},
			]
		}
	}]
	expect: phase: "succeeded"
}

"keeps waiting through two failed attempts": test.#WorkflowStepExec & _cli & {
	_app: "retrying"
	resources: [_job & {
		_app: "retrying"
		status: {failed: 2, startTime: "2026-01-01T00:00:00Z"}
	}]
	expect: phase: "running"
}

"fails once the Job has failed three times": test.#WorkflowStepExec & _cli & {
	_app: "broken"
	resources: [_job & {
		_app: "broken"
		status: {
			failed:    3
			startTime: "2026-01-01T00:00:00Z"
			conditions: [
				{type: "FailureTarget", status: "True", reason: "BackoffLimitExceeded"},
				{type: "Failed", status: "True", reason: "BackoffLimitExceeded"},
			]
		}
	}]
	expect: {
		phase:   "failed"
		message: "failed to execute vela command"
	}
}
