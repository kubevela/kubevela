// We put Components in resources directory.
// References:
// - https://kubevela.net/docs/end-user/components/references
// - https://kubevela.net/docs/platform-engineers/addon/intro#resources-directoryoptional
output: {
	type: "k8s-objects"
	properties: {
		objects: [
			{
				// This creates a plain old Kubernetes namespace
				apiVersion: "v1"
				kind:       "Namespace"
				// We can use the parameter defined in parameter.cue like this.
				metadata: name: parameter.myparam
			},
		]
	}
}
