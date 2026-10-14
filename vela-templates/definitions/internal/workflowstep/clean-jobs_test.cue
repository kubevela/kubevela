import (
	"vela/kube"
	"vela/test"
)

_objects: {
	_name: string
	_labels: {...}
	job: {
		apiVersion: "batch/v1"
		kind:       "Job"
		metadata: {name: _name, labels: _labels}
		spec: template: spec: {
			restartPolicy: "Never"
			containers: [{name: "run", image: "busybox"}]
		}
	}
	pod: {
		apiVersion: "v1"
		kind:       "Pod"
		metadata: {name: _name, labels: _labels}
		spec: containers: [{name: "run", image: "busybox"}]
	}
}

_ours: _objects & {_name: "ours", _labels: "workflow.oam.dev/name": "test-app"}
_theirs: _objects & {_name: "theirs", _labels: {"workflow.oam.dev/name": "other-app", batch: "nightly"}}

// A Job is deleted orphaning its pods, and with no garbage collector to lift
// the orphan finalizer it stays, marked for deletion.
_gone: {
	_name: string
	job: {
		call: kube.#Read & {$params: value: {apiVersion: "batch/v1", kind: "Job", metadata: name: _name}}
		returns: value: metadata: deletionTimestamp: string
	}
	pod: {
		call: kube.#Read & {$params: value: {apiVersion: "v1", kind: "Pod", metadata: name: _name}}
		returns: err: =~"not found"
	}
}

_kept: {
	_name: string
	objects: [{apiVersion: "batch/v1", kind: "Job", metadata: {name: _name, deletionTimestamp?: _|_}}, {apiVersion: "v1", kind: "Pod", metadata: name: _name}]
}

"deletes the application's jobs and pods in the step's namespace": test.#WorkflowStepExec & {
	definition: "clean-jobs"
	resources: [_ours.job, _ours.pod, _theirs.job, _theirs.pod]
	expect: {
		phase: "succeeded"
		checks: _gone & {_name: "ours"}
		resources: (_kept & {_name: "theirs"}).objects
	}
}

"deletes the application's jobs and pods in the given namespace": test.#WorkflowStepExec & {
	definition: "clean-jobs"
	context: namespace:   "clean-jobs-given"
	parameter: namespace: "clean-jobs-given"
	resources: [_ours.job, _ours.pod, _theirs.job, _theirs.pod]
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Delete": [
			{$params: filter: {namespace: "clean-jobs-given", matchingLabels: {"workflow.oam.dev/name": "test-app"} @exact()}},
			{$params: filter: {namespace: "clean-jobs-given", matchingLabels: {"workflow.oam.dev/name": "test-app"} @exact()}},
		]
		checks: _gone & {_name: "ours"}
		resources: (_kept & {_name: "theirs"}).objects
	}
}

"deletes by the given label selector instead": test.#WorkflowStepExec & {
	definition: "clean-jobs"
	context: namespace: "clean-jobs-selector"
	parameter: {namespace: "clean-jobs-selector", labelselector: batch: "nightly"}
	resources: [_ours.job, _ours.pod, _theirs.job, _theirs.pod]
	expect: {
		phase: "succeeded"
		calls: "vela/kube": "#Delete": [
			{$params: filter: {
				matchingLabels: {batch: "nightly"} @exact()
			}},
			{$params: filter: {
				matchingLabels: {batch: "nightly"} @exact()
			}},
		]
		checks: _gone & {_name: "theirs"}
		resources: (_kept & {_name: "ours"}).objects
	}
}
