// Line-level auxiliary in CUE (single object, literal CUE only).
apiVersion: "v1"
kind:       "ConfigMap"
metadata: {
	name: "widget-kit-inline-v1-line-config"
	labels: {
		"kitinline.example.com/tier": "line"
		"kitinline.example.com/line": "v1"
	}
}
data: {
	line:          "v1"
	defaultClass:  "widget-kit-inline-v1-standard"
	configRevision: "1"
}
