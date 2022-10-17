#PatchK8sObject: {
	#do:       "patch-k8s-object"
	#provider: "util"
	value: {...}
	patch: {...}
	result: {...}
	...
}

#String: {
	#do:       "string"
	#provider: "util"

	bt:   bytes
	str?: string
	...
}

#Log: {
	#do:       "log"
	#provider: "util"

	data?: {...} | string
	level: *3 | int
	// note that if you set source in multiple op.#Log, only the latest one will work
	source?: close({
		url: string
	}) | close({
		resources?: [...{
			name?:      string
			cluster?:   string
			namespace?: string
			labelSelector?: {...}
		}]
	})
}
