package module

#Render: {
	#do:       "render"
	#provider: "module"

	$params: {
		module:    string
		registry:  *"" | string
		namespace: *"" | string
		version:   *"" | string
	}
	$returns?: {
		application: {...}
		...
	}
	...
}
