// gadget-kit module-level auxiliary in CUE: a cluster-scoped ClusterRole.
apiVersion: "rbac.authorization.k8s.io/v1"
kind:       "ClusterRole"
metadata: {
	name: "gadget-kit-editor"
	labels: "kit.example.com/tier": "module"
}
rules: [{
	apiGroups: ["kit.example.com"]
	resources: ["gadgets"]
	verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
}]
