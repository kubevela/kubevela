output: {
	type: "helm"
	properties: {
		chart:           "vela-rollout"
		version:         parameter["version"]
		repoType:        "helm"
		url:             "https://charts.kubevela.net/core"
		targetNamespace: "vela-system"
		releaseName:     "vela-rollout"
		values:          parameter["values"]
	}
}
