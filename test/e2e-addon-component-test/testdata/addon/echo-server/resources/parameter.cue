parameter: {
	// +usage=Namespace the echo server is installed into
	namespace: *"echo-system" | string
	// +usage=Container image for the echo server
	image: *"ealen/echo-server:0.9.2" | string
	// +usage=Number of echo server replicas
	replicas: *1 | int
	// +usage=Service type, one of ClusterIP, NodePort or LoadBalancer
	serviceType: *"ClusterIP" | string
	// +usage=Port the echo server listens on
	port: *80 | int
}
