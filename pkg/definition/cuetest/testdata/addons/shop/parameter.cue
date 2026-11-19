parameter: {
	image:       *"shop:1.0" | string
	replicas:    *1 | int
	serviceType: *"ClusterIP" | "NodePort" | "LoadBalancer"
}
