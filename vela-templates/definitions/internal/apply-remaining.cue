import (
  "vela/op",
)
"apply-remaining": {
  type: "workflow-step"
  annotations: {}
  labels: {}
  description: "Apply remaining components and traits"
  attributes: {
    podDisruptive: false
  }
}
template: {
  patch: {
    // apply remaining components and traits
    apply: op.#ApplyRemaining & {
      parameter
    }
  }

  parameter: {
    // +usage=Declare the name of the component
    exceptions?: [componentName=string]: {
      // +usage=skipApplyWorkload indicates whether to skip apply the workload resource
      skipApplyWorkload: *true | bool

      // +usage=skipAllTraits indicates to skip apply all resources of the traits.
      // +usage=If this is true, skipApplyTraits will be ignored
      skipAllTraits: *true| bool
      
      // +usage=skipApplyTraits specifies the names of the traits to skip apply
      skipApplyTraits: [...string]
    }
  }
}
