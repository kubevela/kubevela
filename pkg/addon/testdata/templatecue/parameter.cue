parameter: {

    // global parameters

    // +usage=The namespace of the kube-state-metrics to be installed
    namespace: *"o11y-system" | string
    // +usage=The name of the addon application
    name: *"addon-kube-state-metrics" | string
    // +usage=The clusters to install
    clusters?: [...string]


    // kube-state-metrics parameters

    // +usage=Specify the image of kube-state-metrics
        image: *"bitnami/kube-state-metrics:2.4.2" | string
        // +usage=Specify the imagePullPolicy of the image
        imagePullPolicy: *"IfNotPresent" | "Never" | "Always"
        // +usage=Specify the number of CPU units
        cpu: *0.1 | number
        // +usage=Specifies the attributes of the memory resource required for the container.
        memory: *"250Mi" | string

}