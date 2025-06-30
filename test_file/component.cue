// A simple ConfigMap component with four required keys
// Run:  vela def apply configmap-component.cue
// Then: use `type: configmap-component` in any Application
"configmap-component": {
	alias:        "cm"
  description:  "Creates a ConfigMap with four string values"
  type:         "component"

  attributes: {
  	workload: definition:{
        apiVersion: "v1"
        kind: "ConfigMap"
      }
     }
  }

template: {
	parameter: {
		firstkey:  string
		secondkey:  string
  }

	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name: context.name
		}
		data: {
			firstkey: parameter.firstkey
			secondkey: parameter.secondkey
			data: "57"
		}
	}
}
