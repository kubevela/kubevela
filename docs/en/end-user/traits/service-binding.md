---
title:  Service Binding
---

Service binding trait will bind data from Kubernetes `Secret` to the application container's ENV.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "binding cloud resource secrets to pod env"
  name: service-binding
spec:
  appliesToWorkloads:
    - webservice
    - worker
  schematic:
    cue:
      template: |
        patch: {
        	spec: template: spec: {
        		// +patchKey=name
        		containers: [{
        			name: context.name
        			// +patchKey=name
        			env: [
        				for envName, v in parameter.envMappings {
        					name: envName
        					valueFrom: {
        						secretKeyRef: {
        							name: v.secret
        							if v["key"] != _|_ {
        								key: v.key
        							}
        							if v["key"] == _|_ {
        								key: envName
        							}
        						}
        					}
        				},
        			]
        		}]
        	}
        }

        parameter: {
        	// +usage=The mapping of environment variables to secret
        	envMappings: [string]: [string]: string
        }

```

With the help of this `service-binding` trait, you can explicitly set parameter `envMappings` to mapping all
environment names with secret key. Here is an example.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: webapp
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: zzxwill/flask-web-application:v0.3.1-crossplane
        ports: 80
      traits:
        - type: service-binding
          properties:
            envMappings:
              # environments refer to db-conn secret
              DB_PASSWORD:
                secret: db-conn
                key: password                                     # 1) If the env name is different from secret key, secret key has to be set.
              endpoint:
                secret: db-conn                                   # 2) If the env name is the same as the secret key, secret key can be omitted.
              username:
                secret: db-conn

    - name: sample-db
      type: alibaba-rds
      properties:
        name: sample-db
        engine: mysql
        engineVersion: "8.0"
        instanceClass: rds.mysql.c1.large
        username: oamtest
        secretName: db-conn

```
