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
		resourceTree?: {
			...
		}
	}]
	...
}

#CollectPods: {
	#do:       "collectResources"
	#provider: "query"
	app: {
		name:      string
		namespace: string
		filter?: {
			cluster?:          string
			clusterNamespace?: string
			components?: [...string]
			kind:       "Pod"
			apiVersion: "v1"
		}
		withTree: true
	}
	list: [...{...}]
	...
}

#CollectServices: {
	#do:       "collectResources"
	#provider: "query"
	app: {
		name:      string
		namespace: string
		filter?: {
			cluster?:          string
			clusterNamespace?: string
			components?: [...string]
			kind:       "Service"
			apiVersion: "v1"
		}
		withTree: true
	}
	list: [...{...}]
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
		logs?: string
		err?:  string
		info?: {
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
		withTree: true
	}
	list?: [...{
		endpoint: {
			protocol:     string
			appProtocol?: string
			host?:        string
			port:         int
			portName?:    string
			path?:        string
			inner?:       bool
		}
		ref: {...}
		cluster?:   string
		component?: string
		...
	}]
	...
}

#GetApplicationTree: {
	#do:       "listAppliedResources"
	#provider: "query"
	app: {
		name:      string
		namespace: string
		filter?: {
			cluster?:          string
			clusterNamespace?: string
			components?: [...string]
		}
		withTree: true
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
