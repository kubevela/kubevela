package addon

#Render: {
	#do: "render"
	#provider: "addon"

	$params: {
		addon: string
		version?: string
		registry?: string
		properties?: {...}
	}

	$returns?: {
		resolvedVersion: string
		registry: string
		application: {...}
		resources: [...]
		...
	}
	...
}