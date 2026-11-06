package module

#Render: {
	#do:       "render"
	#provider: "module"

	$params: {
		module:    string
		registry:  *"" | string
		namespace: *"" | string
		version:   *"" | string
		placement?: {
			clusters?: [...string]
			clusterLabelSelector?: [string]: string
			allowEmpty?:           bool
		}
	}
	$returns?: {
		application: {...}
		...
	}
	...
}
