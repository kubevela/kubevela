// gadget-kit-inline crate-level auxiliary in CUE: a cluster-scoped ClusterRole.
apiVersion: "rbac.authorization.k8s.io/v1"
kind:       "ClusterRole"
metadata: {
	name: "gadget-kit-inline-editor"
	labels: "kitinline.example.com/tier": "crate"
}
rules: [{
	apiGroups: ["kitinline.example.com"]
	resources: ["gadgets"]
	verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
}]
