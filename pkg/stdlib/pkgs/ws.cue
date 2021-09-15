#Load: {
	#do:        "load"
	component?: string
	value?: {...}
	...
}

#Export: {
	#do:       "export"
	component: string
	value:     _
}

#DoVar: {
	#do:    "var"
	method: *"Get" | "Put"
	path:   string
	value?: _
}

#Patch: {
	#do: "patch"
	value: {...}
	patch: {...}
}