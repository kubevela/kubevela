output: {
  apiVersion: "apps/v1"
  kind:       "Deployment"
  metadata:
    name: context.name
  spec: {
    replicas: 1

    template: {
      metadata:
        labels:
          "component.oam.dev/name": context.name
          
      spec: {
        containers: [{
          name:  context.name
          image: parameter.image
        }]
      }
    }

    selector: 
      matchLabels:
        "component.oam.dev/name": context.name
  }
}

parameter: {
  // +usage=specify app image
  // +short=i
  image: string
}
