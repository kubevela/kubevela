import "strings"

output: {
	type: "raw"
	properties: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      "alibaba-account-creds"
			namespace: "vela-system"
		}
		type: "Opaque"
		stringData: credentials: strings.Join([creds1, creds2], "\n")
	}
}

creds1: "accessKeyID: " + parameter.ALICLOUD_ACCESS_KEY
creds2: "accessKeySecret: " + parameter.ALICLOUD_SECRET_KEY
