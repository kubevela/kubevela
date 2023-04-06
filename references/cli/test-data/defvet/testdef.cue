// k8s metadata
test1: {
        type:   "trait"
        description: "My test-trait test1"
        attributes: {
                appliesToWorkloads: ["webservice", "worker"]
                podDisruptive: true
        }

}

// template
template: {
        patch: {
                spec: {
                        replicas: *1 | int
                }
        }
        parameter: {
                // +usage=Specify the number of workloads
                replicas: *1 | int
        }
}