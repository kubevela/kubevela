parameter: #route
#route: {
  domain: string
  http: [string]: int
}

// trait template can have multiple outputs and they are all traits
outputs: service: {
  apiVersion: "v1"
  kind: "Service"
  metadata:
    name: context.name
  spec: {
    selector:
      app: context.name
    ports: [
      for k, v in parameter.http {
        port: v
        targetPort: v
      }
    ]
  }
}

outputs: ingress: {
  apiVersion: "networking.k8s.io/v1beta1"
  kind: "Ingress"
  spec: {
    rules: [{
      host: parameter.domain
      http: {
        paths: [
          for k, v in parameter.http {
            path: k
            backend: {
              serviceName: context.name
              servicePort: v
            }
          }
        ]
      }
    }]
  }
}
