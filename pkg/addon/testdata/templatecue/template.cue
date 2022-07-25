output: {
    apiVersion: "core.oam.dev/v1beta1"
    kind: "Application"
    metadata: {
        name: parameter.name
        namespace: "vela-system"
    }
    spec: {
        components: [{
            type: "k8s-objects"
            name: "kube-state-metrics-ns"
            properties: objects: [{
                apiVersion: "v1"
                kind: "Namespace"
                metadata: name: parameter.namespace
            }]
        }]
        policies: [{
            type: "shared-resource"
            name: "namespace"
            properties: rules: [{selector: resourceTypes: ["Namespace"]}]
        }, {
            type: "topology"
            name: "deploy-topology"
            properties: {
                if parameter.clusters != _|_ {
                    clusters: parameter.clusters
                }
                if parameter.clusters == _|_ {
                    clusterLabelSelector: {}
                }
                namespace: parameter.namespace
            }
        }]
    }
}