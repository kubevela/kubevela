import (
	"vela/kube"
	"vela/test"
)

// The step's Job goes to vela-system, which the test cluster lacks.
velaSystem: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "Namespace", metadata: name: "vela-system"}} @before()

// Each case names its own Application, and so its own Job: the API server
// orphans a deleted batch/v1 Job behind a finalizer only the garbage
// collector removes, so a Job cannot be tidied away between cases.
_restart: {
	_app:       string
	definition: "restart-workflow"
	context: {namespace: "restart", appName: _app}
}

// _job is the Job the step applies, running _script, or annotating the
// Application with _value when that is given.
_job: {
	_app:    string
	_value?: string
	_script: string
	if _value != _|_ {
		_script: """
			VALUE="\(_value)"
			kubectl annotate application \(_app) -n restart app.oam.dev/restart-workflow="$VALUE" --overwrite
			"""
	}
	apiVersion: "batch/v1"
	kind:       "Job"
	metadata: {name: "\(_app)-restart-workflow-test-step-id", namespace: "vela-system"}
	spec: {
		backoffLimit: 3
		template: spec: {
			containers: [{
				name:  "kubectl-annotate"
				image: "bitnami/kubectl:latest"
				command: ["/bin/sh", "-c"]
				args: [_script]
			}]
			restartPolicy:      "Never"
			serviceAccountName: "kubevela-vela-core"
		}
	}
}

"runs a Job annotating the Application with the given time": test.#WorkflowStepExec & _restart & {
	_app: "at"
	parameter: at: "2030-01-15T14:30:00Z"
	expect: {
		phase: "running"
		calls: "vela/kube": "#Apply": [{$params: value: metadata: {name: "at-restart-workflow-test-step-id", namespace: "vela-system"}}]
		resources: [_job & {_app: "at", _value: "2030-01-15T14:30:00Z"}]
	}
}

"annotates with the interval to restart every": test.#WorkflowStepExec & _restart & {
	_app: "every"
	parameter: every: "24h"
	expect: {
		phase: "running"
		resources: [_job & {_app: "every", _value: "24h"}]
	}
}

"works out the time to restart after in the Job": test.#WorkflowStepExec & _restart & {
	_app: "after"
	parameter: after: "5m"
	expect: {
		phase: "running"
		checks: script: {
			call: kube.#Read & {$params: value: {apiVersion: "batch/v1", kind: "Job", metadata: {name: "after-restart-workflow-test-step-id", namespace: "vela-system"}}}
			returns: value: spec: template: spec: containers: [{args: [
				=~"DURATION=\"5m\"" & =~"date -u -d" & =~"kubectl annotate application after -n restart app.oam.dev/restart-workflow=\"\\$VALUE\" --overwrite",
			]}]
		}
	}
}

"finishes once the Job has succeeded": test.#WorkflowStepExec & _restart & {
	_app: "done"
	parameter: at: "2030-01-15T14:30:00Z"
	resources: [_job & {
		_app:   "done"
		_value: "2030-01-15T14:30:00Z"
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

"no schedule fails the step": test.#WorkflowStepExec & _restart & {
	_app: "none"
	expect: {
		phase:   "failed"
		message: "Exactly one of 'at', 'after', or 'every' parameters must be specified (found 0)"
		calls: "vela/kube"?: _|_
	}
}

"two schedules fail the step": test.#WorkflowStepExec & _restart & {
	_app: "both"
	parameter: {at: "2030-01-15T14:30:00Z", every: "24h"}
	expect: {
		phase:   "failed"
		message: "Exactly one of 'at', 'after', or 'every' parameters must be specified (found 2)"
		calls: "vela/kube"?: _|_
	}
}

"a duration in another format is rejected": test.#WorkflowStepExec & _restart & {
	_app: "malformed"
	parameter: after: "5 minutes"
	expect: {
		phase:  "failed"
		reason: "Execute"
		calls: "vela/kube"?: _|_
	}
} @pending(the engine applies the Job with after set to 5 minutes and waits on it: a parameter that fails its pattern does not stop the provider calls)
