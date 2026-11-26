// Crate-level auxiliary written in CUE. An auxiliary .cue file is ONE plain
// Kubernetes object at the top level (pkg/crate/parse.go decodeCUEObject),
// not a vela definition wrapper. It is evaluated as literal CUE: there is no
// context or parameter here.
//
// No namespace on purpose: toObjects (pkg/crate/service/render.go) defaults
// it to the namespace the crate's definitions install into (vela-system
// unless the type: crate component sets `namespace`).
apiVersion: "v1"
kind:       "ConfigMap"
metadata: {
	name: "widget-kit-crate-info"
	labels: "kit.example.com/tier": "crate"
}
data: {
	crate:         "widget-kit"
	crateVersion:  "1.0.0"
	servedLines:    "v1,v2"
	disabledLines:  "v1beta1"
	renderedFrom:   "auxiliary/10-crate-info.cue"
}
