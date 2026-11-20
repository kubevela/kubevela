import (
	"vela/test"
	"vela/kube"
)

_cm: {apiVersion: "v1", kind: "ConfigMap"}

"checks what the step left": test.#WorkflowStepExec & {
	definition: "../steps/publish"
	context: namespace: "checks"
	parameter: {name: "cfg", data: {a: "1", next: "pointer"}}
	resources: [
		_cm & {metadata: name: "pointer", data: note: "found by following cfg"},
	]
	expect: {
		phase: "succeeded"
		checks: {
			"cfg has a": {
				call:    kube.#Read & {$params: value: _cm & {metadata: {name: "cfg", namespace: "checks"}}}
				returns: value: data: a: "1"
			}
			"old config is gone": {
				call:    kube.#Read & {$params: value: _cm & {metadata: {name: "old", namespace: "checks"}}}
				returns: err: =~"not found"
			}
			"follows cfg to the next": {
				call: kube.#Read & {$params: value: _cm & {metadata: {
					name:      checks["cfg has a"].call.$returns.value.data.next
					namespace: "checks"
				}}}
				returns: value: data: note: "found by following cfg"
			}
		}
	}
}

"checks ignore the case's mocks": test.#WorkflowStepExec & {
	definition: "../steps/publish"
	context: namespace: "checks-mocked"
	parameter: {name: "real", data: source: "cluster"}
	mocks: "vela/kube": "#Read": $returns: value: data: source: "mock"
	expect: checks: real: {
		call:    kube.#Read & {$params: value: _cm & {metadata: {name: "real", namespace: "checks-mocked"}}}
		returns: value: data: source: "cluster"
	}
}

"an unnamed namespace is the case's": test.#WorkflowStepExec & {
	definition: "../steps/publish"
	parameter: {name: "here", data: a: "1"}
	expect: checks: here: {
		call:    kube.#Read & {$params: value: _cm & {metadata: name: "here"}}
		returns: value: data: a: "1"
	}
}

_web: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {name: "web", labels: {app: "web", tier: "front"}}
	spec: {
		selector: matchLabels: app: "web"
		template: {
			metadata: labels: app: "web"
			spec: containers: [{name: "web", image: "nginx:1.27"}, {name: "proxy", image: "envoy:1.30"}]
		}
	}
}

_readWeb: kube.#Read & {$params: value: {apiVersion: "apps/v1", kind: "Deployment", metadata: name: "web"}}

"matcher attributes work in checks": test.#WorkflowStepExec & {
	definition: "../steps/publish"
	parameter: {name: "cfg", data: a: "1"}
	resources: [_web]
	expect: checks: {
		exact: {
			call: _readWeb
			returns: value: metadata: labels: {app: "web", tier: "front"} @exact()
		}
		not: {
			call: _readWeb
			returns: value: metadata: labels: tier: "back" @not()
		}
		contains: {
			call: _readWeb
			returns: value: spec: template: spec: containers: [{name: "proxy"}, {image: =~"^nginx"}] @contains()
		}
		"contains none": {
			call: _readWeb
			returns: value: spec: template: spec: containers: [{name: "sidecar"}] @contains() @not()
		}
		"optional bottom": {
			call: _readWeb
			returns: value: metadata: annotations?: _|_
		}
		// A missing object is in err: value echoes the object asked for.
		"absent is err, not value": {
			call: kube.#Read & {$params: value: {apiVersion: "v1", kind: "ConfigMap", metadata: name: "nope"}}
			returns: {
				err: =~"not found"
				value: metadata: name: "nope"
			}
		}
	}
}
