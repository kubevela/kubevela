#Apply: {
	#do:       "apply"
	#provider: "kube"
	cluster:   *"" | string
	value: {...}
	...
}

#Read: {
	#do:       "read"
	#provider: "kube"
	value?: {...}
	...
}
