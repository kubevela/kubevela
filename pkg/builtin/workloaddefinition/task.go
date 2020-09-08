package workloaddefinition

var Task = `apiVersion: v1
kind: WorkloadDefinition
metadata:
  name: task
  annotations:
    definition.oam.dev/apiVersion: "v1"
    definition.oam.dev/kind: "Job"
spec:
  definitionRef:
    name: jobs  
  extension:
	defaultTraits:
        - monitor
		- logging
    template: |
      #Template: {
      	apiVersion: "v1"
      	kind:       "Job"
      	metadata: name: task
      	spec: {
			parallelism: taskSpec.count
			completions: taskSpec.count
			template:
			  spec:
				containers: [{
			      image: taskSpec.image
				  name:  taskSpec.name
				    ports: [{
					  containerPort: taskSpec.port
					  protocol:      "TCP"
					  name:          "default"
					}]
				}]
      	}
      }
      taskSpec: {		
		// +usage=specify number of tasks to run in parallel
      	// +short=c
      	count: *1 | int
      	name: string
      	// +usage=specify app image
      	// +short=i
      	image: string
      	// +usage=specify port for container
      	// +short=p
      	port: *6379 | int
      }
`
