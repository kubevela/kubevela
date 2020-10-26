output: {
  apiVersion: "core.oam.dev/v1alpha2"
  kind:       "ManualScalerTrait"
  spec: {
    replicaCount: parameter.replica
  }
}
#scale: {
  //+short=r
  replica: *1 | int
}
parameter: #scale