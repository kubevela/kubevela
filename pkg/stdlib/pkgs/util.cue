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

	data: {...}
}