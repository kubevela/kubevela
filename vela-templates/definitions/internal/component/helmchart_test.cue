import "vela/test"

// Rendering a chart fetches it, so helm.#Render is mocked.
_chart: {
	definition: "helmchart"
	context: {name: "redis", appName: "shop", namespace: "team-a"}
	_source: *"redis" | string
	parameter: chart: {source: _source, repoURL: "https://charts.example.com", version: "18.1.0"}
	_resources: *[
		{apiVersion: "apps/v1", kind: "StatefulSet", metadata: name: "redis"},
		{apiVersion: "v1", kind: "Service", metadata: name: "redis"},
	] | [...]
	mocks: "vela/helm": "#Render": $returns: resources: _resources
}

"records the release, and renders the chart's resources as outputs": test.#ComponentRender & _chart & {
	expect: {
		output: {
			apiVersion: "v1"
			kind:       "ConfigMap"
			metadata: {
				name:      "redis-helm-release"
				namespace: "team-a"
				labels: {"app.oam.dev/name": "shop", "app.oam.dev/component": "redis", "helm.oam.dev/chart": "redis"}
				annotations: "helm.oam.dev/chart": "redis"
			}
			data: {
				chartSource:      "redis"
				releaseName:      "redis"
				releaseNamespace: "team-a"
				repoURL:          "https://charts.example.com"
				chartVersion:     "18.1.0"
				resourceCount:    "2"
			}
		}
		outputs: {
			"helm-resource-0": {kind: "StatefulSet", metadata: name: "redis"}
			"helm-resource-1": {kind: "Service", metadata: name: "redis"}
		} @exact()
	}
}

"the release defaults to the component, the cache to the Application and component": test.#ComponentRender & _chart & {
	expect: calls: "vela/helm": "#Render": [{$params: {
		chart: {source: "redis", version: "18.1.0"}
		release: {name: "redis", namespace: "team-a"}
		options: cache: key: "shop-redis"
		context: {appName: "shop", appNamespace: "team-a", name: "redis", namespace: "team-a"}
	}}]
}

"a named release keeps the Application's namespace": test.#ComponentRender & _chart & {
	parameter: release: name: "cache"
	expect: {
		output: metadata: name: "cache-helm-release"
		calls: "vela/helm": "#Render": [{$params: release: {name: "cache", namespace: "team-a"}}]
	}
}

"values and a cache key pass through": test.#ComponentRender & _chart & {
	parameter: {
		values: replicas: 3
		options: cache: key: "redis-pinned"
	}
	expect: calls: "vela/helm": "#Render": [{$params: {values: replicas: 3, options: cache: key: "redis-pinned"}}]
}

"a chart source that is not a plain name is not a label": test.#ComponentRender & _chart & {
	_source: "oci://registry.example.com/charts/redis"
	expect: output: metadata: {
		labels: "helm.oam.dev/chart"?:     _|_
		annotations: "helm.oam.dev/chart": "oci://registry.example.com/charts/redis"
	}
}

"a chart with nothing to install has no outputs": test.#ComponentRender & _chart & {
	_resources: []
	expect: {
		output: data: resourceCount: "0"
		outputs: {} @exact()
	}
}

"with no health criteria it is deployed": test.#ComponentStatus & _chart & {
	expect: {healthy: true, message: "Deployed"}
}

"healthy when every criterion's condition holds": test.#ComponentStatus & _chart & {
	parameter: healthStatus: [{resource: {kind: "StatefulSet", name: "redis"}, condition: type: "Ready"}]
	observed: outputs: "helm-resource-0": status: conditions: [{type: "Ready", status: "True"}]
	expect: {healthy: true, message: "Deployed"}
}

"not healthy while a condition does not hold": test.#ComponentStatus & _chart & {
	parameter: healthStatus: [{resource: {kind: "StatefulSet", name: "redis"}, condition: type: "Ready"}]
	observed: outputs: "helm-resource-0": status: conditions: [{type: "Ready", status: "False"}]
	expect: {healthy: false, message: "Deploying"}
}

"not healthy when no rendered resource matches a criterion": test.#ComponentStatus & _chart & {
	parameter: healthStatus: [{resource: {kind: "Deployment", name: "redis"}, condition: type: "Available"}]
	expect: healthy: false
}

"a criterion without a name matches any resource of its kind": test.#ComponentStatus & _chart & {
	parameter: healthStatus: [{resource: kind: "StatefulSet", condition: type: "Ready"}]
	observed: outputs: "helm-resource-0": status: conditions: [{type: "Ready", status: "True"}]
	expect: healthy: true
}

"a criterion without a name is not met by another kind": test.#ComponentStatus & _chart & {
	parameter: healthStatus: [{resource: kind: "Deployment", condition: type: "Available"}]
	observed: outputs: "helm-resource-0": status: conditions: [{type: "Ready", status: "True"}]
	expect: healthy: false
}

"every rendering option passes through, the cache key still defaulted": test.#ComponentRender & _chart & {
	parameter: options: {
		includeCRDs:     false
		skipTests:       false
		skipHooks:       true
		createNamespace: false
		timeout:         "10m"
		maxHistory:      3
		atomic:          true
		wait:            true
		force:           true
		recreatePods:    true
		cleanupOnFail:   true
	}
	expect: calls: "vela/helm": "#Render": [{$params: options: {
		includeCRDs:     false
		skipTests:       false
		skipHooks:       true
		createNamespace: false
		timeout:         "10m"
		maxHistory:      3
		atomic:          true
		wait:            true
		force:           true
		recreatePods:    true
		cleanupOnFail:   true
		cache: key: "shop-redis"
	}}]
}

"chart credentials, a release namespace and values sources pass through": test.#ComponentRender & _chart & {
	parameter: {
		chart: auth: secretRef: {name: "charts-login", namespace: "cache"}
		release: {name: "cache", namespace: "cache"}
		valuesFrom: [
			{kind: "ConfigMap", name: "redis-defaults"},
			{kind: "Secret", name: "redis-auth", namespace: "cache", key: "values.yaml", optional: true},
		]
	}
	expect: {
		output: metadata: {name: "cache-helm-release", namespace: "cache"}
		calls: "vela/helm": "#Render": [{$params: {
			chart: auth: secretRef: {name: "charts-login", namespace: "cache"}
			release: {name: "cache", namespace: "cache"}
			valuesFrom: [
				{kind: "ConfigMap", name: "redis-defaults", optional?: _|_},
				{kind: "Secret", name: "redis-auth", namespace: "cache", key: "values.yaml", optional: true},
			]
		}}]
	}
}
