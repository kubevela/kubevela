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

#CollectLogsInPod: {
	#do:       "collectLogsInPod"
	#provider: "query"
	cluster:   string
	namespace: string
	pod:       string
	options: {
		container:    string
		previous:     *false | bool
		sinceSeconds: *null | int
		sinceTime:    *null | string
		timestamps:   *false | bool
		tailLines:    *null | int
		limitBytes:   *null | int
	}
	outputs?: {
		logs: string
		err?: string
		info: {
			fromDate: string
			toDate:   string
		}
		...
	}
	...
}

#CollectServiceEndpoints: {
	#do:       "collectServiceEndpoints"
	#provider: "query"
	app: {
		name:      string
		namespace: string
		filter?: {
			cluster?:          string
			clusterNamespace?: string
		}
	}
	list?: [...{
		endpoint: {
			protocol:    string
			appProtocol: string
			host?:       string
			port:        int
			path?:       string
		}
		ref: {...}
	}]
	...
}
