"my-plc": {
    description: "My service mesh policy."
    type:        "policy"
}

template: {
    #ServerWeight: {
        service: string
        weight:  int
    }
    parameter: {
        weights: [...#ServerWeight]
    }

    output: {
        apiVersion: "policy/v1"
        kind:       "PodDisruptionBudget1"
        metadata: name: context.name
        spec: {
            service:  context.name
            backends: parameter.weights
        }
    }
}