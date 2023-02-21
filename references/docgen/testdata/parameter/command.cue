#PatchParams: {
	// +usage=Specify the name of the target container, if not set, use the component name
	containerName: *"" | string
	// +usage=Specify the command to use in the target container, if not set, it will not be changed
	command: *null | [...string]
	// +usage=Specify the args to use in the target container, if set, it will override existing args
	args: *null | [...string]
	// +usage=Specify the args to add in the target container, existing args will be kept, cannot be used with args
	addArgs: *null | [...string]
	// +usage=Specify the existing args to delete in the target container, cannot be used with args
	delArgs: *null | [...string]
}

parameter: #PatchParams | close({
	// +usage=Specify the commands for multiple containers
	containers: [...#PatchParams]
})
