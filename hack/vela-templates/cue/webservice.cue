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

          if parameter["cmd"] != _|_ {
            command: parameter.cmd
          }

          if parameter["env"] != _|_ {
            env: parameter.env
          }
          
          if context["config"] != _|_ {
            env: [
              for k, v in context.config {
                name: k
                value: v
              }
            ]
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

  cmd?: [...string]

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
