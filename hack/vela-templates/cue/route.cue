output: {
  apiVersion: "standard.oam.dev/v1alpha1"
  kind:       "Route"
  spec: {
    host: parameter.domain

    if parameter.issuer != "" {
      tls: {
        issuerName: parameter.issuer
      }
    }

    rules: parameter.rules
  }
}
#route: {
  domain: *"" | string
  issuer: *"" | string
  rules: [...{
    path: string
    rewriteTarget: *"" | string
  }]
}
parameter: #route
