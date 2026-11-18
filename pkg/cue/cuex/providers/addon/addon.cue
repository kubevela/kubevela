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
		// Where the installer wants the addon. Absent means the addon places
		// itself, if at all.
		placement?: {
			clusters?: [...string]
			clusterLabelSelector?: [string]: string
			allowEmpty?: bool
		}
	}
	$returns?: {
		resolvedVersion: string
		registry:        string
		application: {...}
		...
	}
	...
}
