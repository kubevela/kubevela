"ref-objects": {
	type: "component"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "Ref-objects allow users to specify ref objects to use. Notice that this component type have special handle logic."
	attributes: {
		workload: type: "autodetects.core.oam.dev"
		status: {
			customStatus: #"""
				if context.output.apiVersion == "apps/v1" && context.output.kind == "Deployment" {
					ready: {
						readyReplicas: *0 | int
					} & {
						if context.output.status.readyReplicas != _|_ {
							readyReplicas: context.output.status.readyReplicas
						}
					}
					message: "Ready:\(ready.readyReplicas)/\(context.output.spec.replicas)"
				}
				if context.output.apiVersion != "apps/v1" || context.output.kind != "Deployment" {
					message: ""
				}
				"""#
			healthPolicy: #"""
				if context.output.apiVersion == "apps/v1" && context.output.kind == "Deployment" {
					ready: {
						updatedReplicas:    *0 | int
						readyReplicas:      *0 | int
						replicas:           *0 | int
						observedGeneration: *0 | int
					} & {
						if context.output.status.updatedReplicas != _|_ {
							updatedReplicas: context.output.status.updatedReplicas
						}
						if context.output.status.readyReplicas != _|_ {
							readyReplicas: context.output.status.readyReplicas
						}
						if context.output.status.replicas != _|_ {
							replicas: context.output.status.replicas
						}
						if context.output.status.observedGeneration != _|_ {
							observedGeneration: context.output.status.observedGeneration
						}
					}
					isHealth: (context.output.spec.replicas == ready.readyReplicas) && (context.output.spec.replicas == ready.updatedReplicas) && (context.output.spec.replicas == ready.replicas) && (ready.observedGeneration == context.output.metadata.generation || ready.observedGeneration > context.output.metadata.generation)
				}
				if context.output.apiVersion != "apps/v1" || context.output.kind != "Deployment" {
					isHealth: true
				}
				"""#
		}
	}
}
template: {
	#K8sObject: {
		// +usage=The resource type for the Kubernetes objects
		resource?: string
		// +usage=The group name for the Kubernetes objects
		group?: string
		// +usage=If specified, fetch the Kubernetes objects with the name, exclusive to labelSelector
		name?: string
		// +usage=If specified, fetch the Kubernetes objects from the namespace. Otherwise, fetch from the application's namespace.
		namespace?: string
		// +usage=If specified, fetch the Kubernetes objects from the cluster. Otherwise, fetch from the local cluster.
		cluster?: string
		// +usage=If specified, fetch the Kubernetes objects according to the label selector, exclusive to name
		labelSelector?: [string]: string
		...
	}

	output: {
		if len(parameter.objects) > 0 {
			parameter.objects[0]
		}
		...
	}

	outputs: {
		for i, v in parameter.objects {
			if i > 0 {
				"objects-\(i)": v
			}
		}
	}
	parameter: {
		// +usage=If specified, application will fetch native Kubernetes objects according to the object description
		objects?: [...#K8sObject]
		// +usage=If specified, the objects in the urls will be loaded.
		urls?: [...string]
	}
}
