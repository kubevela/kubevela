// Line-level auxiliary in CUE (single object, literal CUE only).
apiVersion: "v1"
kind:       "ConfigMap"
metadata: {
	name: "widget-kit-v1-line-config"
	labels: {
		"kit.example.com/tier": "line"
		"kit.example.com/line": "v1"
	}
}
data: {
	line:          "v1"
	defaultClass:  "widget-kit-v1-standard"
	configRevision: "2"
}
