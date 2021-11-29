"register-grafana-datasource": {
	annotations: {}
	attributes: {
		appliesToWorkloads: []
		conflictsWith: []
		podDisruptive:   false
		workloadRefPath: ""
	}
	description: "Add a datasource to Grafana"
	labels: {
		"ui-hidden": "true"
	}
	type: "trait"
}

template: {
	outputs: registerdatasource: {
		apiVersion: "grafana.extension.oam.dev/v1alpha1"
		kind:       "DatasourceRegistration"
		spec: {
			grafana: {
				service:                   parameter.grafanaServiceName
				namespace:                 parameter.grafanaServiceNamespace
				credentialSecret:          parameter.credentialSecret
				credentialSecretNamespace: parameter.credentialSecretNamespace
			}
			datasource: {
				name:      parameter.name
				type:      parameter.type
				access:    parameter.access
				service:   parameter.service
				namespace: parameter.namespace
			}
		}
	}
	parameter: {
		grafanaServiceName:        string
		grafanaServiceNamespace:   *"default" | string
		credentialSecret:          string
		credentialSecretNamespace: string
		name:                      string
		type:                      string
		access:                    *"proxy" | string
		service:                   string
		namespace:                 *"default" | string
	}

}
