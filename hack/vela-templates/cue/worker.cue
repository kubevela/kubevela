output: {
  apiVersion: "standard.oam.dev/v1alpha1"
  kind:       "PodSpecWorkload"
  metadata:
    name: context.name
  spec: {
    replicas: 1
    podSpec: {
      containers: [{
        image: parameter.image
        name:  context.name
      }]
    }
  }
}

parameter: {
  // +usage=specify app image
  // +short=i
  image: string
}
