import (
	"vela/kube"
	"vela/test"
)

// A platform Application runs the step in vela-system, which the test cluster lacks.
velaSystem: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "Namespace", metadata: name: "vela-system"}} @before()

// The accounts the cases run as. kubevela-vela-core exists only in vela-system, as in a cluster.
velaCLINamespace: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "Namespace", metadata: name: "vela-cli"}} @before()
defaultAccount: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "ServiceAccount", metadata: {name: "default", namespace: "vela-cli"}}} @before()
deployerAccount: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "ServiceAccount", metadata: {name: "deployer", namespace: "vela-cli"}}} @before()
controllerAccount: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "ServiceAccount", metadata: {name: "kubevela-vela-core", namespace: "vela-system"}}} @before()

// Each case names its own Application, and so its own Job: the API server
// orphans a deleted batch/v1 Job behind a finalizer only the garbage
// collector removes, so a Job cannot be tidied away between cases.
_cli: {
	_app:       string
	definition: "vela-cli"
	context: {namespace: *"vela-cli" | string, appName: _app}
	parameter: command: ["vela", "addon", "enable", "fluxcd"]
}

// _job is the Job the step applies with the default parameters: in the Application's
// namespace, as its default service account.
_job: {
	_app:       string
	apiVersion: "batch/v1"
	kind:       "Job"
	metadata: {name: "\(_app)-test-step-test-step-id", namespace: "vela-cli"}
	spec: {
		backoffLimit: 3
		template: {
			metadata: labels: "workflow.oam.dev/step-name": "\(_app)-test-step"
			spec: {
				containers: [{
					name:  "\(_app)-test-step-test-step-id-job"
					image: "oamdev/vela-cli:v1.12.0-alpha.2"
					command: ["vela", "addon", "enable", "fluxcd"]
					// The API server drops the step's empty lists.
					volumeMounts?: _|_
				}]
				restartPolicy:      "Never"
				serviceAccountName: "default"
				volumes?:           _|_
			}
		}
	}
}

"runs the command in a Job and waits for it": test.#WorkflowStepExec & _cli & {
	_app: "enable"
	expect: {
		phase: "running"
		calls: {
			"vela/kube": "#Apply": [{$params: value: metadata: {name: "enable-test-step-test-step-id", namespace: "vela-cli"}}]
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
		metadata: {name: "secrets-test-step-test-step-id", namespace: "vela-cli"}
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

"mounts a host path": test.#WorkflowStepExec & _cli & {
	_app: "host"
	parameter: storage: hostPath: [{name: "docker", path: "/var/run/docker.sock", mountPath: "/var/run/docker.sock", type: "Socket"}]
	expect: resources: [{
		apiVersion: "batch/v1"
		kind:       "Job"
		metadata: {name: "host-test-step-test-step-id", namespace: "vela-cli"}
		spec: template: spec: {
			containers: [{volumeMounts: [{name: "hostpath-docker", mountPath: "/var/run/docker.sock"}]}]
			volumes: [{name: "hostpath-docker", hostPath: {path: "/var/run/docker.sock", type: "Socket"}}]
		}
	}]
}

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

"a host path is a directory by default": test.#WorkflowStepExec & _cli & {
	_app: "hostdir"
	parameter: storage: hostPath: [{name: "cache", path: "/var/cache/vela", mountPath: "/cache"}]
	expect: resources: [{
		apiVersion: "batch/v1"
		kind:       "Job"
		metadata: {name: "hostdir-test-step-test-step-id", namespace: "vela-cli"}
		spec: template: spec: volumes: [{name: "hostpath-cache", hostPath: {path: "/var/cache/vela", type: "Directory"}}]
	}]
}

// Naming the controller's service account does not reach the system namespace: the Job would run
// where the Application is, and an account of that name is not there, so the step fails.
"stays in the Application's namespace whatever service account is named": test.#WorkflowStepExec & _cli & {
	_app: "controller-sa"
	parameter: serviceAccountName: "kubevela-vela-core"
	expect: {
		phase:   "failed"
		message: "service account kubevela-vela-core not found in namespace vela-cli: the vela-cli Job runs as it"
	}
}

// A Job as a missing account would create no pod and never finish, so the step fails at once.
"fails when the service account is not in the Application's namespace": test.#WorkflowStepExec & _cli & {
	_app: "no-sa"
	parameter: serviceAccountName: "missing"
	expect: {
		phase:   "failed"
		message: "service account missing not found in namespace vela-cli: the vela-cli Job runs as it"
	}
}

// A platform Application in vela-system can still run the step as the controller's account.
"a platform Application runs it in vela-system": test.#WorkflowStepExec & _cli & {
	_app: "platform"
	context: namespace:            "vela-system"
	parameter: serviceAccountName: "kubevela-vela-core"
	expect: resources: [{
		apiVersion: "batch/v1"
		kind:       "Job"
		metadata: {name: "platform-test-step-test-step-id", namespace: "vela-system"}
		spec: template: spec: serviceAccountName: "kubevela-vela-core"
	}]
}
