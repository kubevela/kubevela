#ListResourcesInApp: {
	#do:       "listResourcesInApp"
	#provider: "query"
	app: {
		name:      string
		namespace: string
		components?: [...string]
		filter?: {
			cluster?:          string
			clusterNamespace?: string
		}
		clusterNamespace?:   string
		enableHistoryQuery?: bool
	}
	...
}

#CollectPods: {
	#do:       "collectPods"
	#provider: "query"
	value: {...}
	cluster: string
	...
}

#SearchEvents: {
	#do:       "searchEvents"
	#provider: "query"
	value: {...}
	cluster: string
	...
}
