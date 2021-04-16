patch: {
	spec: replicas: parameter.replicas
}
parameter: {
	// +usage=Specify the number of workload
	replicas: *1 | int
}
