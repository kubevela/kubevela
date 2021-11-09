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
	cluster:   *"" | string
	value?: {...}
	...
}

#List: {
	#do:       "list"
	#provider: "kube"
	cluster:   *"" | string
	resource: {
		apiVersion: string
		kind:       string
	}
	filter?: {
		namespace?: *"" | string
		matchingLabels?: {...}
	}
	list?: {...}
}

#Delete: {
	#do:       "delete"
	#provider: "kube"
	cluster:   *"" | string
	value: {
		apiVersion: string
		kind:       string
		metadata: {
			name:      string
			namespace: *"default" | string
		}
	}
	...
}
