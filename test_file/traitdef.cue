// HPA trait metadata
myhpa: {
  type:        "trait"
  annotations: {}
  description: "Autoscale the component based on CPU utilization."
  attributes: {
    // apply only to Deployments (as created by containerized-service)
    appliesToWorkloads: ["deployments.apps"]
  }
}

// Rendering logic + parameter schema
template: {
  outputs: myhpa: {
    apiVersion: "autoscaling/v2"
    kind:       "HorizontalPodAutoscaler"
    metadata: {
      name: context.name
    }
    spec: {
      scaleTargetRef: {
        apiVersion: parameter.targetAPIVersion
        kind:       parameter.targetKind
        name:       context.name
      }
      minReplicas: parameter.minReplicas
      maxReplicas: parameter.maxReplicas
      metrics: [{
        type: "Resource"
        resource: {
          name: "cpu"
          target: {
            type:               "Utilization"
            averageUtilization: parameter.cpu
          }
        }
      }]
    }
  }

  parameter: {
    // +usage=Minimal number of replicas
    minReplicas: *3  | int
    // +usage=Maximum number of replicas
    maxReplicas: *10 | int
    // +usage=Target CPU utilization percentage (e.g. 50 for 50%)
    cpu:     *60 | int

    // +usage=APIVersion of the scale target
    targetAPIVersion?: *"apps/v1"  | string
    // +usage=Kind of the scale target
    targetKind?:       *"Deployment" | string
  }
}
