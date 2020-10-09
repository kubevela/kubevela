parameter: #webservice
#webservice: {
  // +vela:cli:enbaled=true
  // +vela:cli:usage=specify commands to run in container
  // +vela:cli:short=c
  cmd: [...string]

  env: [...string]

  files: [...string]
}

output: {
  apiVersion: "apps/v1"
  kind: "Deployment"
  metadata:
    name: context.name
  spec: {
    selector: {
      matchLabels:
        app: context.name
    }
    template: {
      metadata:
        labels:
          app: context.name
      spec: {
        containers: [{
          name:  context.name
          image: context.image
          command: parameter.cmd
        }]
      }
    }
  }
}
