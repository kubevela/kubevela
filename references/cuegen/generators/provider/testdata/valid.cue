package test

#Apply: {
	#do:       "apply"
	#provider: "test"
	$params: {
		// +usage=The cluster to use
		cluster: string
		// +usage=The resource to get or apply
		resource: {
			...
		}
		// +usage=The options to get or apply
		options: {
			// +usage=The strategy of the resource
			threeWayMergePatch: {
				// +usage=The strategy to get or apply the resource
				enabled: *true | bool
				// +usage=The annotation prefix to use for the three way merge patch
				annotationPrefix: *"resource" | string
			}
		}
	}
	$returns: {
		...
	}
}

#Get: {
	#do:       "get"
	#provider: "test"
	$params: {
		// +usage=The cluster to use
		cluster: string
		// +usage=The resource to get or apply
		resource: {
			...
		}
		// +usage=The options to get or apply
		options: {
			// +usage=The strategy of the resource
			threeWayMergePatch: {
				// +usage=The strategy to get or apply the resource
				enabled: *true | bool
				// +usage=The annotation prefix to use for the three way merge patch
				annotationPrefix: *"resource" | string
			}
		}
	}
	$returns: {
		...
	}
}

#List: {
	#do:       "list"
	#provider: "test"
	$params: {
		// +usage=The cluster to use
		cluster: string
		// +usage=The filter to list the resources
		filter?: {
			// +usage=The namespace to list the resources
			namespace?: string
			// +usage=The label selector to filter the resources
			matchingLabels?: [string]: string
		}
		// +usage=The resource to list
		resource: {
			...
		}
	}
	$returns: {
		...
	}
}

#Patch: {
	#do:       "patch"
	#provider: "test"
	$params: {
		// +usage=The cluster to use
		cluster: string
		// +usage=The resource to patch
		resource: {
			...
		}
		// +usage=The patch to be applied to the resource with kubernetes patch
		patch: {
			// +usage=The type of patch being provided
			type: "merge" | "json" | "strategic"
			data: _
		}
	}
	$returns: {
		...
	}
}
