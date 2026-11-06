package addon

#Render: {
	#do:       "render"
	#provider: "addon"

	$params: {
		addon:    string
		version:  *"" | string
		registry: *"" | string
		properties: {...}
		skipVersionValidate: *false | bool
		// Namespace of the Application that installs the addon; the addon's
		// modules install their definitions into it.
		namespace: *"" | string
	}
	$returns?: {
		resolvedVersion: string
		registry:        string
		application: {...}
		...
	}
	...
}
