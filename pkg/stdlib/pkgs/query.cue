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
			kind?:       string
			apiVersion?: string
		}
		withStatus?: bool
	}
	list?: [...{
		cluster:   string
		component: string
		revision:  string
		object: {...}
	}]
	...
}

#ListAppliedResources: {
	#do:       "listAppliedResources"
	#provider: "query"
	app: {
		name:      string
		namespace: string
		filter?: {
			cluster?:          string
			clusterNamespace?: string
			components?: [...string]
			kind?:       string
			apiVersion?: string
		}
	}
	list?: [...{
		name:             string
		namespace?:       string
		cluster?:         string
		component?:       string
		trait?:           string
		kind?:            string
		uid?:             string
		apiVersion?:      string
		resourceVersion?: string
		publishVersion?:  string
		deployVersion?:   string
		revision?:        string
		latest?:          bool
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
			components?: [...string]
		}
	}
	list?: [...{
		endpoint: {
			protocol:     string
			appProtocol?: string
			host?:        string
			port:         int
			path?:        string
		}
		ref: {...}
		cluster?:   string
		component?: string
		...
	}]
	...
}

#GetApplicationTree: {
	#do:       "getApplicationTree"
	#provider: "query"
	app: {
		name:      string
		namespace: string
	}
	list?: [...{
		name:             string
		namespace?:       string
		cluster?:         string
		component?:       string
		trait?:           string
		kind?:            string
		uid?:             string
		apiVersion?:      string
		resourceVersion?: string
		publishVersion?:  string
		deployVersion?:   string
		revision?:        string
		latest?:          bool
		...
	}]
	...
}
