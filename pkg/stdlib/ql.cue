#Read: {
	#do:       "read"
	#provider: "kube"

	// +usage=The cluster to use
	cluster: *"" | string
	// +usage=The resource to read, this field will be filled with the resource read from the cluster after the action is executed
	value?: {...}
	...
}

#List: {
	#do:       "list"
	#provider: "kube"

	// +usage=The cluster to use
	cluster: *"" | string
	// +usage=The resource to list
	resource: {
		// +usage=The api version of the resource
		apiVersion: string
		// +usage=The kind of the resource
		kind: string
	}
	// +usage=The filter to list the resources
	filter?: {
		// +usage=The namespace to list the resources
		namespace?: *"" | string
		// +usage=The label selector to filter the resources
		matchingLabels?: {...}
	}
	// +usage=The listed resources will be filled in this field after the action is executed
	list?: {...}
	...
}

#Delete: {
	#do:       "delete"
	#provider: "kube"

	// +usage=The cluster to use
	cluster: *"" | string
	// +usage=The resource to delete
	value: {
		// +usage=The api version of the resource
		apiVersion: string
		// +usage=The kind of the resource
		kind: string
		// +usage=The metadata of the resource
		metadata: {
			// +usage=The name of the resource
			name?: string
			// +usage=The namespace of the resource
			namespace: *"default" | string
		}
	}
	// +usage=The filter to delete the resources
	filter?: {
		// +usage=The namespace to list the resources
		namespace?: string
		// +usage=The label selector to filter the resources
		matchingLabels?: {...}
	}
	...
}

#ListResourcesInApp: query.#ListResourcesInApp

#ListAppliedResources: query.#ListAppliedResources

#CollectPods: query.#CollectPods

#CollectServices: query.#CollectServices

#SearchEvents: query.#SearchEvents

#CollectLogsInPod: query.#CollectLogsInPod

#CollectServiceEndpoints: query.#CollectServiceEndpoints

#GetApplicationTree: query.#GetApplicationTree
