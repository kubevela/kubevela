// Module-level auxiliary written in CUE. An auxiliary .cue file is ONE plain
// Kubernetes object at the top level (pkg/module/parse.go decodeCUEObject),
// not a vela definition wrapper. It is evaluated as literal CUE: there is no
// context or parameter here.
//
// No namespace on purpose: toObjects (pkg/module/service/render.go) defaults
// it to the namespace the module's definitions install into (vela-system
// unless the type: module component sets `namespace`).
apiVersion: "v1"
kind:       "ConfigMap"
metadata: {
	name: "widget-kit-inline-module-info"
	labels: "kitinline.example.com/tier": "module"
}
data: {
	module:         "widget-kit-inline"
	moduleVersion:  "1.0.0"
	servedLines:    "v1,v2"
	disabledLines:  "v1beta1"
	renderedFrom:   "auxiliary/10-module-info.cue"
}
