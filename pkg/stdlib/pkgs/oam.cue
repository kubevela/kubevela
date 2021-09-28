#ApplyComponent: {
	#provider: "oam"
	#do:       "component-apply"
	cluster:   *"" | string
	value: {...}
	patch?: {...}
	...
}

#LoadComponets: {
	#provider: "oam"
	#do:       "load"
	...
}
