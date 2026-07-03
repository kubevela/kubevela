// component-db-provisioner.cue
//
// Demonstrates upgrade rule 1: list-arithmetic
//
// Pre-v1.11 syntax uses `+` to concatenate lists and `*` to repeat them.
// CUE v0.14+ treats both as hard errors. The upgrade engine rewrites them
// at render time so existing stored definitions continue to work.
//
// Broken lines (pre-v1.11):
//   allEnv:          parameter.env + [{name: "MANAGED_BY", value: "kubevela"}]
//   expandedScripts: parameter.initScripts * parameter.scriptReplicas
//
// After upgrade:
//   allEnv:          list.Concat([parameter.env, [{name: "MANAGED_BY", value: "kubevela"}]])
//   expandedScripts: list.Repeat(parameter.initScripts, parameter.scriptReplicas)

"db-provisioner": {
	type:        "component"
	description: "Provisions a database Deployment. Uses legacy list concatenation to merge user-supplied env vars with platform defaults."
	attributes: workload: {
		definition: {
			apiVersion: "apps/v1"
			kind:       "Deployment"
		}
		type: "deployments.apps"
	}
}

template: {
	// ISSUE 1 (list-arithmetic): pre-v1.11 list concatenation via + and repetition via *
	// Rewritten at render time to list.Concat([...]) and list.Repeat(...)
	allEnv:          parameter.env + [{name: "MANAGED_BY", value: "kubevela"}]
	expandedScripts: parameter.initScripts * parameter.scriptReplicas

	output: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: context.name
		spec: {
			replicas: parameter.replicas
			selector: matchLabels: "app.oam.dev/component": context.name
			template: {
				metadata: labels: "app.oam.dev/component": context.name
				spec: {
					initContainers: [{
						name:    "db-init"
						image:   "busybox:1.35"
						command: expandedScripts
					}]
					containers: [{
						name:  context.name
						image: parameter.image
						env:   allEnv
						ports: [{containerPort: parameter.port}]
					}]
				}
			}
		}
	}

	parameter: {
		// +usage=Database container image
		// +short=i
		image: *"postgres:15" | string

		// +usage=Number of database replicas
		replicas: *1 | int

		// +usage=Service port
		port: *5432 | int

		// +usage=Extra environment variables to inject
		env: *[] | [...{name: string, value?: string}]

		// +usage=Init container script commands
		initScripts: *["echo", "init"] | [...string]

		// +usage=How many times to repeat the init script list
		scriptReplicas: *1 | int
	}
}
