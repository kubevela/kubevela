import "vela/kube"

"vela-addon": {
	type: "source"
	annotations: {}
	labels: {}
	description: "Whether a KubeVela addon is installed and running, and the name and namespace of the Application it built - chain into vela-app for the detail."
}

template: {
	schema: {
		installed: bool
		running:   bool
		app:       string
		namespace: string
	}

	storage: {
		storageTTL:     "1m"
		onStaleFailure: "fail"
	}

	parameter: {
		// +usage=Name of the addon, as `vela addon list` reports it
		name: string
		// +usage=Namespace addon Applications are installed into. Set it where the chart was installed with a non-default systemDefinitionNamespace.
		namespace: *"vela-system" | string
	}

	_addonNamespace: parameter.namespace

	_apps: kube.#List & {
		$params: {
			resource: {
				apiVersion: "core.oam.dev/v1beta1"
				kind:       "Application"
			}
			filter: {
				namespace: _addonNamespace
				matchingLabels: "addons.oam.dev/name": parameter.name
			}
		}
	}

	_items: *_apps.$returns.items | []
	_found: len(_items) > 0

	output: {
		installed: _found
		running: [
			if _found if (*_items[0].status.status | "") == "running" {true},
			false,
		][0]
		app: [
			if _found {_items[0].metadata.name},
			"",
		][0]
		namespace: _addonNamespace
	}
}
