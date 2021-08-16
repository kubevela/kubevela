import (
  "vela/op",
)
"multi-env": {
  type: "workflow-step"
  annotations: {}
  labels: {}
  description: "Apply env binding component"
  attributes: {
    podDisruptive: false
  }
}
template: {
  patch: {
    component: op.#ApplyEnvBindComponent & {
      env:       parameter.env
      policy:    parameter.policy
      component: parameter.component
      // context.namespace indicates the namespace of the app
      namespace: context.namespace
    }
  }

  parameter: {
    // +usage=Declare the name of the component
    component: string
    // +usage=Declare the name of the policy
    policy: string
    // +usage=Declare the name of the env in policy
    env: string
  }
}
