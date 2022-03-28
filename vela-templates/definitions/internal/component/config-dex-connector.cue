"config-dex-connector": {
	type: "component"
	annotations: {
		"alias.config.oam.dev": "Dex Connector"
	}
	labels: {
		"catalog.config.oam.dev":       "velacore-config"
		"type.config.oam.dev":          "dex-connector"
		"multi-cluster.config.oam.dev": "false"
	}
	description: "Config information to authenticate Dex connectors"
	attributes: workload: type: "autodetects.core.oam.dev"
}

template: {
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      parameter.name
			namespace: context.namespace
			labels: {
				"config.oam.dev/catalog":       "velacore-config"
				"config.oam.dev/type":          "dex-connector"
				"config.oam.dev/multi-cluster": "false"
				"config.oam.dev/identifier":    parameter.name
				"config.oam.dev/sub-type":      parameter.type
			}
		}
		type: "Opaque"

		if parameter.type == "github" {
			stringData: parameter.github
		}
		if parameter.type == "ldap" {
			stringData: parameter.ldap
		}
	}

	parameter: {
		// +usage=Config type
		type: "github" | "ldap"
		github?: {
			// +usage=GitHub client ID
			clientID: string
			// +usage=GitHub client secret
			clientSecret: string
			// +usage=GitHub call back URL
			callbackURL: string
		}
		ldap?: {
			host:               string
			insecureNoSSL:      *true | bool
			insecureSkipVerify: bool
			startTLS:           bool
			usernamePrompt:     string
			userSearch: {
				baseDN:    string
				username:  string
				idAttr:    string
				emailAttr: string
				nameAttr:  string
			}
		}
	}
}
