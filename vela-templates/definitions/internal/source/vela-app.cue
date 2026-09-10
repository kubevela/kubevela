import "vela/kube"

"vela-app": {
	type: "source"
	annotations: {}
	labels: {}
	description: "The status of another KubeVela Application - phase, overall health, workflow state, and per-component status broken down by the cluster each is placed in. Status only; never its spec."
}

template: {
	schema: {
		name:      string
		namespace: string
		phase:     string
		healthy:   bool
		revision?: string
		workflow?: {
			mode:       string
			phase:      string
			suspend:    bool
			terminated: bool
			finished:   bool
			message:    string
		}
		components: [...string]
		clusters: [...string]
		services: [string]: {
			healthy: bool
			clusters: [string]: {
				healthy:   bool
				message:   string
				namespace: string
			}
		}
	}

	storage: {
		storageTTL:     "1m"
		onStaleFailure: "fail"
	}

	parameter: {
		// +usage=Name of the Application to read
		name: string
		// +usage=Namespace it lives in. Defaults to the consuming Application's own.
		namespace?: string
	}

	_ns: [
		if parameter.namespace != _|_ {parameter.namespace},
		context.namespace,
	][0]

	_app: kube.#Get & {
		$params: resource: {
			apiVersion: "core.oam.dev/v1beta1"
			kind:       "Application"
			metadata: {
				name:      parameter.name
				namespace: _ns
			}
		}
	}

	_status: *_app.$returns.status | {}
	_services: *_status.services | []

	_placed: [for s in _services {
		name: s.name
		cluster: [if (*s.cluster | "") != "" {*s.cluster | ""}, "local"][0]
		healthy:   s.healthy
		message:   *s.message | ""
		namespace: *s.namespace | ""
	}]

	_names: [for n in {for p in _placed {"\(p.name)": p.name}} {n}]
	_clusters: [for c in {for p in _placed {"\(p.cluster)": p.cluster}} {c}]

	output: {
		name:      parameter.name
		namespace: _ns
		phase:     *_status.status | "unknown"
		healthy: len(_services) > 0 && len([for s in _services if !s.healthy {s}]) == 0
		if _status.latestRevision != _|_ {
			revision: _status.latestRevision.name
		}
		if _status.workflow != _|_ {
			workflow: {
				mode:       *_status.workflow.mode | ""
				phase:      *_status.workflow.status | "unknown"
				suspend:    *_status.workflow.suspend | false
				terminated: *_status.workflow.terminated | false
				finished:   *_status.workflow.finished | false
				message:    *_status.workflow.message | ""
			}
		}
		components: _names
		clusters:   _clusters
		services: {
			for n in _names {
				"\(n)": {
					healthy: len([for p in _placed if p.name == n if !p.healthy {p}]) == 0
					clusters: {
						for p in _placed if p.name == n {
							"\(p.cluster)": {
								healthy:   p.healthy
								message:   p.message
								namespace: p.namespace
							}
						}
					}
				}
			}
		}
	}
}
