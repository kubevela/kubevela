import "vela/velaconfig"

"vela-config": {
	type: "source"
	annotations: {}
	labels: {}
	description: "Reads a KubeVela Config: its properties, the ConfigTemplate it satisfies, and references to the Secret and named objects that template produced. Output values are never returned, only their references. A Config marked sensitive is refused."
}

template: {
	schema: {
		// +sensitive
		properties: _
		template: {
			name:       string
			namespace?: string
		}
		output: {
			apiVersion: string
			kind:       string
			name:       string
			namespace?: string
		}
		outputs: [string]: {
			apiVersion: string
			kind:       string
			name:       string
			namespace?: string
		}
	}

	storage: {
		storageTTL:     "5m"
		onStaleFailure: "use-stale"
	}

	parameter: {
		// +usage=Name of the Config
		name: string
		// +usage=Namespace it lives in. Defaults to vela-system.
		namespace?: string
	}

	_cfg: velaconfig.#Read & {
		$params: {
			name: parameter.name
			if parameter.namespace != _|_ {
				namespace: parameter.namespace
			}
		}
	}

	output: {
		properties: _cfg.$returns.properties
		template:   _cfg.$returns.template
		output:     _cfg.$returns.output
		outputs:    _cfg.$returns.outputs
	}
}
