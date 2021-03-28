# Definition Docs

## Reserved word
### Patch
Perform the CUE AND operation with the content declared by 'patch' and workload cr,

you can define the strategy of list merge through comments, example as follows

base model
 ```
containers: [{
        name: "x1"
}, {
        name: "x2"
        image:  string
        envs: [{
                name: "OPS"
                value: string
        }, ...]
}, ...]
```
define patch model
```
 // +patchKey=name
containers: [{
        name: "x2"
        image:  "test-image"
        envs: [{
                name: "OPS1"
                value: "V-OPS1"
        },{
                name: "OPS"
                value: "V-OPS"
        }, ...]
}, ...]
```
and the result model after patch is follow
```
containers: [{
        name: "x1" 
 },{
        name: "x2"
        image:  "test-image"
        envs: [{
                name: "OPS1"
                value: "V-OPS1"
        },{
                name: "OPS"
                value: "V-OPS"
        }, ...]
}, ...]
 ```


### output
Generate a new cr, which is generally associated with workload cr

## ComponentDefinition
The following ComponentDefinition is to generate a deployment
```
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    cue:
      template: |
        output: {
          apiVersion: "apps/v1"
          kind:       "Deployment"
          spec: {
            selector: matchLabels: {
              "app.oam.dev/component": context.name
            }

            template: {
              metadata: labels: {
                "app.oam.dev/component": context.name
              }

              spec: {
                containers: [{
                  name:  context.name
                  image: parameter.image

                  if parameter["cmd"] != _|_ {
                    command: parameter.cmd
                  }
                }]
              }
            }
          }
        }

        parameter: {
          // +usage=Which image would you like to use for your service
          // +short=i
          image: string

          cmd?: [...string]
        }
```

If defined an application as follows
```
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: application-sample
spec:
  components:
    - name: myweb
      type: worker
      properties:
        image: "busybox"
        cmd:
        - sleep
        - "1000"
```
we will get a deployment
```
apiVersion: apps/v1
kind: Deployment
spec:
  selector:
    matchLabels:
      app.oam.dev/component: myweb
  template:
    metadata:
      labels:
        app.oam.dev/component: myweb
    spec:
      containers:
      - command:
        - sleep
        - "1000"
        image: busybox
        name: myweb
```
##  Service Trait Definition

Define a trait Definition that appends service to workload(worker) , as shown below
```
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "service the app"
  name: kservice
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |-
        patch: {spec: template: metadata: labels: app: context.name}
        outputs: service: {
          apiVersion: "v1"
          kind: "Service"
          metadata: name: context.name
          spec: {
            selector:  app: context.name
            ports: [
              for k, v in parameter.http {
                port: v
                targetPort: v
              }
            ]
          }
        }
        parameter: {
          http: [string]: int
        }
```

If add service capability to the application, as follows
```
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: application-sample
spec:
  components:
    - name: myweb
      type: worker
      properties:
        image: "busybox"
        cmd:
        - sleep
        - "1000"
      traits:
        - type: kservice
          properties:
            http:
              server: 80
```

we will get a new deployment and service
```
// origin deployment  template add labels
apiVersion: apps/v1
kind: Deployment
spec:
  selector:
    matchLabels:
      app.oam.dev/component: myweb
  template:
    metadata:
      labels:
        // add label app
        app: myweb
        app.oam.dev/component: myweb
    spec:
      containers:
      - command:
        - sleep
        - "1000"
        image: busybox
        name: myweb
---
apiVersion: v1
kind: Service
metadata:
  name: myweb
spec:
  ports:
  - port: 80
    targetPort: 80
  selector:
    app: myweb         
```

## Scaler Trait Definition

Define a trait Definition that scale workload(worker) replicas
```
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Manually scale the app"
  name: scaler
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |-
        patch: {
          spec: replicas: parameter.replicas
        }
        parameter: {
          //+short=r
          replicas: *1 | int
        }
```
If add scaler capability to the application, as follows
```
  components:
    - name: myweb
      type: worker
      properties:
        image: "busybox"
        cmd:
        - sleep
        - "1000"
      traits:
        - type: kservice
          properties:
            http:
              server: 80           
        - type: scaler
          properties:
            replicas: 10
```

The deployment replicas will be scale to 10
```
apiVersion: apps/v1
kind: Deployment
spec:
  selector:
    matchLabels:
      app.oam.dev/component: myweb
  // scale to 10    
  replicas: 10    
  template:
    metadata:
      labels:
        // add label app
        app: myweb
        app.oam.dev/component: myweb
    spec:
      containers:
      - command:
        - sleep
        - "1000"
        image: busybox
        name: myweb
```

## Sidecar Trait Definition

Define a trait Definition that append containers to workload(worker)
```
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "add sidecar to the app"
  name: sidecar
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |-
        patch: {
          // +patchKey=name
          spec: template: spec: containers: [parameter]
        }
        parameter: {
          name: string
          image: string
          command?: [...string]
        }
```

If add sidercar capability to the application, as follows
```
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: application-sample
spec:
  components:
    - name: myweb
      type: worker
      properties:
        image: "busybox"
        cmd:
        - sleep
        - "1000"
      traits:
        - type: scaler
          properties:
            replicas: 10
        - type: sidecar
          properties:
            name: "sidecar-test"
            image: "nginx"
        - type: kservice
          properties:
            http:
              server: 80
```
The deployment updated as follows
```
apiVersion: apps/v1
kind: Deployment
spec:
  selector:
    matchLabels:
      app.oam.dev/component: myweb
  // scale to 10    
  replicas: 10    
  template:
    metadata:
      labels:
        // add label app
        app: myweb
        app.oam.dev/component: myweb
    spec:
      containers:
      - command:
        - sleep
        - "1000"
        image: busybox
        name: myweb
      - name: sidecar-test
        image: nginx  
```
