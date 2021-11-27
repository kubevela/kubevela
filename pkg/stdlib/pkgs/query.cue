#ListResourcesInApp: {
	#do:       "listResourcesInApp"
	#provider: "query"
	app: {
		name:      string
		namespace: string
		filter?: {
			cluster?:          string
			clusterNamespace?: string
			components?: [...string]
		}
	}
	list?: [...{
		cluster:   string
		component: string
		revision:  string
		object: {...}
	}]
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
