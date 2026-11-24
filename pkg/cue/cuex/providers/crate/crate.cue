package crate

#Render: {
	#do:       "render"
	#provider: "crate"

	$params: {
		crate:    string
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
