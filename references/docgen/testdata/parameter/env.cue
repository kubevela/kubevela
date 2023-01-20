#PatchParams: {
	// +usage=Specify the name of the target container, if not set, use the component name
	containerName: *"" | string
	// +usage=Specify if replacing the whole environment settings for the container
	replace: *false | bool
	// +usage=Specify the  environment variables to merge, if key already existing, override its value
	env: [string]: string
	// +usage=Specify which existing environment variables to unset
	unset: *[] | [...string]
}
parameter: #PatchParams | close({
	// +usage=Specify the environment variables for multiple containers
	containers: [...#PatchParams]
})
