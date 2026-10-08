apiVersion: "v1"
kind:       "ConfigMap"
metadata: {
	name: "gadget-kit-v1-profile"
	labels: {
		"kit.example.com/tier": "line"
		"kit.example.com/line": "v1"
	}
}
data: profiles: "eco,turbo"
