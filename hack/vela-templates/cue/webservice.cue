output: {
  apiVersion: "apps/v1"
  kind:       "Deployment"
  metadata: name: context.name
  spec: {
    replicas: 1

    selector: matchLabels: {
      "app.oam.dev/component": context.name
    }

    template: {
      metadata: labels: {
        "app.oam.dev/component": context.name
      }

      spec: {
        containers: [{
          name:  context.name
          image: parameter.image
          if parameter["env"] != _|_ {
            env: parameter.env
          }
          ports: [{
            containerPort: parameter.port
          }]
        }]
      }
    }
  }
}
parameter: {
  // +usage=specify app image
  // +short=i
  image: string

  // +usage=specify port for container
  // +short=p
  port:  *6379 | int

  env?: [...{
    name:  string
    value?: string
    valueFrom?: {
      secretKeyRef: {
        name: string
        key: string
      }
    }
  }]
}
