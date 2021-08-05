kustomize: {
	type: "component"
	annotations: {}
	labels: {}
	description: "kustomize can fetching, building, updating and applying Kustomize manifests from git repo."
	attributes: workload: type: "autodetects.core.oam.dev"
}
template: {
	output: {
		apiVersion: "source.toolkit.fluxcd.io/v1beta1"
		kind:       "GitRepository"
		metadata: name: context.name
		spec: {
			interval: parameter.pullInterval
			url:      parameter.repoUrl
			ref: branch: parameter.branch
		}
	}
	outputs: kustomize: {
		apiVersion: "kustomize.toolkit.fluxcd.io/v1beta1"
		kind:       "Kustomization"
		metadata: name: context.name
		spec: {
			interval: parameter.pullInterval
			sourceRef: {
				kind: "GitRepository"
				name: context.name
			}
			path:       parameter.path
			prune:      true
			validation: "client"
		}
	}

	parameter: {
		//+usage=The repository URL, can be a HTTP/S or SSH address.
		repoUrl: string

		//+usage=The interval at which to check for repository updates.
		pullInterval: *"1m" | string

		//+usage=The Git reference to checkout and monitor for changes, defaults to master branch.
		branch: *"master" | string

		//+usage=Path to the directory containing the kustomization.yaml file, or the set of plain YAMLs a kustomization.yaml should be generated for.
		path: string
	}
}
