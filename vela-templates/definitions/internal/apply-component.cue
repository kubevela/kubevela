import (
  "vela/op",
)
"apply-component": {
  type: "workflow-step"
  annotations: {}
  labels: {}
  description: "Apply components and traits for your workflow steps"
}
template: {
  patch: {
    // apply components and traits
    apply: op.#ApplyComponent & {
      component: parameter.component
    }
  }

  parameter: {
    // +usage=Declare the name of the component
    component: string
	}
}
