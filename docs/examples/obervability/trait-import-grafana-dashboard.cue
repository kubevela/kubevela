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
		url: parameter.url
	}
}

parameter: {
	grafanaServiceName:        string
	grafanaServiceNamespace:   *"default" | string
	credentialSecret:          string
	credentialSecretNamespace: *"default" | string
	url:                       string
}
