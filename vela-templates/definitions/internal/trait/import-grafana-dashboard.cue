"import-grafana-dashboard": {
	attributes: {
		appliesToWorkloads: []
		conflictsWith: []
		podDisruptive:   false
		workloadRefPath: ""
	}
	description: "Import dashboards to Grafana"
	labels: {
		"ui-hidden": "true"
	}
	type: "trait"
}

template: {
	outputs: registerdatasource: {
		apiVersion: "grafana.extension.oam.dev/v1alpha1"
		kind:       "ImportDashboard"
		spec: {
			grafana: {
				service:                   parameter.grafanaServiceName
				namespace:                 parameter.grafanaServiceNamespace
				credentialSecret:          parameter.credentialSecret
				credentialSecretNamespace: parameter.credentialSecretNamespace
			}
			urls: parameter.urls
		}
	}
	parameter: {
		grafanaServiceName:        string
		grafanaServiceNamespace:   *"default" | string
		credentialSecret:          string
		credentialSecretNamespace: *"default" | string
		urls: [...string]

	}
}
