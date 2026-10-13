apiVersion: "v1"
kind:       "ConfigMap"
metadata: {
	name: "gadget-kit-inline-v1-profile"
	labels: {
		"kitinline.example.com/tier": "line"
		"kitinline.example.com/line": "v1"
	}
}
data: profiles: "eco,turbo"
