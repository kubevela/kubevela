import "vela/test"

_cm: {apiVersion: "v1", kind: "ConfigMap", metadata: name: "settings", data: tier: "gold"}
_sa: {apiVersion: "v1", kind: "ServiceAccount", metadata: name: "shop"}

"the first object is the output, as given": test.#ComponentRender & {
	definition: "k8s-objects"
	parameter: objects: [_cm]
	expect: {
		output: _cm
		outputs: {} @exact()
	}
}

"the rest are outputs, numbered from one": test.#ComponentRender & {
	definition: "k8s-objects"
	parameter: objects: [_cm, _sa, {apiVersion: "v1", kind: "ServiceAccount", metadata: name: "worker"}]
	expect: {
		outputs: {
			"objects-1": _sa
			"objects-2": metadata: name: "worker"
		} @exact()
	}
}
