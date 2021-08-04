outputs: registerdatasource: {
	apiVersion: "grafana.extension.oam.dev/v1alpha1"
	kind:       "DatasourceRegistration"
	spec: {
		grafanaUrl:    parameter.grafanaUrl
		credentialSecret:     parameter.credentialSecret
		credentialSecretNamespace: parameter.credentialSecretNamespace
		name:          parameter.name
		url:           parameter.url
		type:          parameter.type
		access:        parameter.access
	}
}

parameter: {
	grafanaUrl:    string
	credentialSecret:     string
	credentialSecretNamespace: string
	name:          string
	url:           string
	type:          string
	access:        *"proxy" | string
}
