output: {
	type: "raw"
	properties: {
		apiVersion: "terraform.core.oam.dev/v1beta1"
		kind:       "Provider"
		metadata: {
			name:      "default"
			namespace: "default"
		}
		spec: {
			provider: "alibaba"
			region:   parameter.ALICLOUD_REGION
			credentials: {
				source: "Secret"
				secretRef: {
					namespace: "vela-system"
					name:      "alibaba-account-creds"
					key:       "credentials"
				}
			}
		}
	}
}
