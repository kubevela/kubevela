/*
 Copyright 2021. The KubeVela Authors.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package plugin

import (
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/e2e"
)

var _ = Describe("Test Kubectl Plugin", func() {
	namespace := "default"
	componentDefName := "test-webservice"
	traitDefName := "test-ingress"

	Context("Test kubectl vela dry-run", func() {
		It("Test dry-run application use definitions which applied to the cluster", func() {
			By("check definitions which application used whether applied to the cluster")
			var cd v1beta1.ComponentDefinition
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentDefName}, &cd)
				return err
			}, 5*time.Second, time.Second).Should(BeNil())

			var td v1beta1.TraitDefinition
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: traitDefName}, &td)
				return err
			}, 5*time.Second, time.Second).Should(BeNil())

			By("dry-run application")
			err := os.WriteFile("dry-run-app.yaml", []byte(application), 0644)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() string {
				output, _ := e2e.Exec("kubectl-vela dry-run -f dry-run-app.yaml -n vela-system")
				return output
			}, 10*time.Second, time.Second).Should(ContainSubstring(dryRunResult))
		})

		It("Test dry-run application use definitions in local", func() {
			Eventually(func() string {
				output, _ := e2e.Exec("kubectl-vela dry-run -f dry-run-app.yaml -d definitions")
				return output
			}, 10*time.Second, time.Second).Should(ContainSubstring(dryRunResult))
		})
	})

	Context("Test kubectl vela live-diff", func() {
		applicationName := "test-vela-app"

		It("Test live-diff application use definition which applied to the cluster", func() {
			By("check definitions which application used whether applied to the cluster")
			var cd v1beta1.ComponentDefinition
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentDefName}, &cd)
				return err
			}, 5*time.Second).Should(BeNil())

			var td v1beta1.TraitDefinition
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: traitDefName}, &td)
				return err
			}, 5*time.Second).Should(BeNil())

			By("get appRevision")
			var appRev v1beta1.ApplicationRevision
			var appRevName = fmt.Sprintf("%s-v1", applicationName)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: appRevName}, &appRev)
				return err
			}, 5*time.Second).Should(BeNil())

			Eventually(func() bool {
				var tempApp v1beta1.Application
				_ = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: app.Name}, &tempApp)
				return tempApp.Status.LatestRevision != nil
			}, 20*time.Second, time.Second).Should(BeTrue())

			By("live-diff application")
			err := os.WriteFile("live-diff-app.yaml", []byte(newApplication), 0644)
			Expect(err).NotTo(HaveOccurred())
			output, err := e2e.Exec("kubectl-vela live-diff -f live-diff-app.yaml")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).Should(ContainSubstring(livediffResult))
		})

		It("Test dry-run application use definitions in local", func() {
			Eventually(func() bool {
				var tempApp v1beta1.Application
				_ = k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: app.Name}, &tempApp)
				return tempApp.Status.LatestRevision != nil
			}, 20*time.Second, time.Second).Should(BeTrue())

			output, err := e2e.Exec("kubectl-vela live-diff -f live-diff-app.yaml -d definitions")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).Should(ContainSubstring(livediffResult))
		})
	})

	Context("Test kubectl vela show", func() {
		It("Test show componentDefinition reference", func() {
			cdName := "test-show-task"
			output, err := e2e.Exec(fmt.Sprintf("kubectl-vela show %s -n default", cdName))
			Expect(err).NotTo(HaveOccurred())
			Expect(output).Should(ContainSubstring(showCdResult))
		})
		It("Test show traitDefinition reference", func() {
			tdName := "test-sidecar"
			output, err := e2e.Exec(fmt.Sprintf("kubectl-vela show %s -n default", tdName))
			Expect(err).NotTo(HaveOccurred())
			Expect(output).Should(ContainSubstring(showTdResult))
		})
		It("Test show componentDefinition use Helm Charts as Workload", func() {
			Eventually(func() string {
				cdName := "test-webapp-chart"
				output, _ := e2e.Exec(fmt.Sprintf("kubectl-vela show %s -n default", cdName))
				return output
			}, 20*time.Second, time.Second).Should(ContainSubstring("Specification"))
		})
		It("Test show componentDefinition def with raw Kube mode", func() {
			cdName := "kube-worker"
			output, err := e2e.Exec(fmt.Sprintf("kubectl-vela show %s -n default", cdName))
			Expect(err).NotTo(HaveOccurred())
			Expect(output).Should(ContainSubstring("image"))
			Expect(output).Should(ContainSubstring("The value will be applied to fields: [spec.template.spec.containers[0].image]."))
			Expect(output).Should(ContainSubstring("port"))
			Expect(output).Should(ContainSubstring("the specific container port num which can accept external request."))
		})
		It("Test show traitDefinition def with raw Kube mode", func() {
			tdName := "service-kube"
			output, err := e2e.Exec(fmt.Sprintf("kubectl-vela show %s -n default", tdName))
			Expect(err).NotTo(HaveOccurred())
			Expect(output).Should(ContainSubstring("targetPort"))
			Expect(output).Should(ContainSubstring("target port num for service provider."))
		})
		It("Test show traitDefinition def with cue single map parameter", func() {
			tdName := "annotations"
			output, err := e2e.Exec(fmt.Sprintf("kubectl-vela show %s -n default", tdName))
			Expect(err).NotTo(HaveOccurred())
			Expect(output).Should(ContainSubstring("map[string]:(null|string)"))
		})
		It("Test show webservice def with cue ignore annotation ", func() {
			tdName := "webservice"
			output, err := e2e.Exec(fmt.Sprintf("kubectl-vela show %s -n default", tdName))
			Expect(err).NotTo(HaveOccurred())
			Expect(output).ShouldNot(ContainSubstring("addRevisionLabel"))
		})
		It("Test show webservice def with cue ignore annotation ", func() {
			tdName := "mywebservice"
			output, err := e2e.Exec(fmt.Sprintf("kubectl-vela show %s -n default", tdName))
			Expect(err).NotTo(HaveOccurred())
			Expect(output).ShouldNot(ContainSubstring("addRevisionLabel"))
			Expect(output).ShouldNot(ContainSubstring("mySecretKey"))
		})
	})
})

var application = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: test-vela-app
  namespace: default
spec:
  components:
    - name: express-server
      type: test-webservice
      properties:
        image: crccheck/hello-world
        port: 80
      traits:
        - type: test-ingress
          properties:
            domain: testsvc.example.com
            http:
              "/": 80
`

var newApplication = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: test-vela-app
  namespace: default
spec:
  components:
    - name: new-express-server
      type: test-webservice
      properties:
        image: crccheck/hello-world
        port: 5000
        cpu: "0.5"
      traits:
        - type: test-ingress
          properties:
            domain: new-testsvc.example.com
            http:
              "/": 8080
`

var componentDef = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: test-webservice
  namespace: default
  annotations:
    definition.oam.dev/description: "Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers."
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
        				if parameter.addRevisionLabel {
        					"app.oam.dev/appRevision": context.appRevision
        				}
        			}
        
        			spec: {
        				containers: [{
        					name:  context.name
        					image: parameter.image
        
        					if parameter["cmd"] != _|_ {
        						command: parameter.cmd
        					}
        
        					if parameter["env"] != _|_ {
        						env: parameter.env
        					}
        
        					if context["config"] != _|_ {
        						env: context.config
        					}
        
        					ports: [{
        						containerPort: parameter.port
        					}]
        
        					if parameter["cpu"] != _|_ {
        						resources: {
        							limits:
        								cpu: parameter.cpu
        							requests:
        								cpu: parameter.cpu
        						}
        					}
        
        					if parameter["volumes"] != _|_ {
        						volumeMounts: [ for v in parameter.volumes {
        							{
        								mountPath: v.mountPath
        								name:      v.name
        							}}]
        					}
        				}]
        
        			if parameter["volumes"] != _|_ {
        				volumes: [ for v in parameter.volumes {
        					{
        						name: v.name
        						if v.type == "pvc" {
        							persistentVolumeClaim: {
        								claimName: v.claimName
        							}
        						}
        						if v.type == "configMap" {
        							configMap: {
        								defaultMode: v.defaultMode
        								name:        v.cmName
        								if v.items != _|_ {
        									items: v.items
        								}
        							}
        						}
        						if v.type == "secret" {
        							secret: {
        								defaultMode: v.defaultMode
        								secretName:  v.secretName
        								if v.items != _|_ {
        									items: v.items
        								}
        							}
        						}
        						if v.type == "emptyDir" {
        							emptyDir: {
        								medium: v.medium
        							}
        						}
        					}}]
        			}
        		}
        		}
        	}
        }
        parameter: {
        	// +usage=Which image would you like to use for your service
        	// +short=i
        	image: string
        
        	// +usage=Commands to run in the container
        	cmd?: [...string]
        
        	// +usage=Which port do you want customer traffic sent to
        	// +short=p
        	port: *80 | int
        	// +usage=Define arguments by using environment variables
        	env?: [...{
        		// +usage=Environment variable name
        		name: string
        		// +usage=The value of the environment variable
        		value?: string
        		// +usage=Specifies a source the value of this var should come from
        		valueFrom?: {
        			// +usage=Selects a key of a secret in the pod's namespace
        			secretKeyRef: {
        				// +usage=The name of the secret in the pod's namespace to select from
        				name: string
        				// +usage=The key of the secret to select from. Must be a valid secret key
        				key: string
        			}
        		}
        	}]

        	cpu?: string

        	// If addRevisionLabel is true, the appRevision label will be added to the underlying pods
        	addRevisionLabel: *false | bool

        	// +usage=Declare volumes and volumeMounts
        	volumes?: [...{
        		name:      string
        		mountPath: string
        		// +usage=Specify volume type, options: "pvc","configMap","secret","emptyDir"
        		type: "pvc" | "configMap" | "secret" | "emptyDir"
        		if type == "pvc" {
        			claimName: string
        		}
        		if type == "configMap" {
        			defaultMode: *420 | int
        			cmName:      string
        			items?: [...{
        				key:  string
        				path: string
        				mode: *511 | int
        			}]
        		}
        		if type == "secret" {
        			defaultMode: *420 | int
        			secretName:  string
        			items?: [...{
        				key:  string
        				path: string
        				mode: *511 | int
        			}]
        		}
        		if type == "emptyDir" {
        			medium: *"" | "Memory"
        		}
        	}]
        }

`

var componentDefWithHelm = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: test-webapp-chart
  namespace: default
  annotations:
    definition.oam.dev/description: helm chart for webapp
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    helm:
      release:
        chart:
          spec:
            chart: "podinfo"
            version: "5.1.4"
      repository:
        url: "https://charts.kubevela.net/example/"
`

var componentDefWithKube = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: kube-worker
  namespace: default
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    kube:
      template:
        apiVersion: apps/v1
        kind: Deployment
        spec:
          selector:
            matchLabels:
              app: nginx
          template:
            metadata:
              labels:
                app: nginx
            spec:
              containers:
                - name: nginx
                  ports:
                    - containerPort: 80
      parameters:
        - name: image
          required: true
          type: string
          fieldPaths:
            - "spec.template.spec.containers[0].image"
        - name: port
          required: true
          type: string
          fieldPaths:
            - "spec.template.spec.containers[0].ports[0].containerPort"
          description: "the specific container port num which can accept external request."
`

var traitDef = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Configures K8s ingress and service to enable web traffic for your service.
    Please use route trait in cap center for advanced usage."
  name: test-ingress
  namespace: default
spec:
  status:
    customStatus: |-
      if len(context.outputs.ingress.status.loadBalancer.ingress) > 0 {
      	message: "Visiting URL: " + context.outputs.ingress.spec.rules[0].host + ", IP: " + context.outputs.ingress.status.loadBalancer.ingress[0].ip
      }
      if len(context.outputs.ingress.status.loadBalancer.ingress) == 0 {
      	message: "No loadBalancer found, visiting by using 'vela port-forward " + context.appName + " --route'\n"
      }
    healthPolicy: |
      isHealth: len(context.outputs.service.spec.clusterIP) > 0
  appliesToWorkloads:
    - deployments.apps
  podDisruptive: false
  schematic:
    cue:
      template: |
        parameter: {
        	domain: string
        	http: [string]: int
        }
        
        // trait template can have multiple outputs in one trait
        outputs: service: {
        	apiVersion: "v1"
        	kind:       "Service"
        	metadata:
        		name: context.name
        	spec: {
        		selector: {
        			"app.oam.dev/component": context.name
        		}
        		ports: [
        			for k, v in parameter.http {
        				port:       v
        				targetPort: v
        			},
        		]
        	}
        }
        
        outputs: ingress: {
        	apiVersion: "networking.k8s.io/v1beta1"
        	kind:       "Ingress"
        	metadata:
        		name: context.name
        	spec: {
        		rules: [{
        			host: parameter.domain
        			http: {
        				paths: [
        					for k, v in parameter.http {
        						path: k
        						backend: {
        							serviceName: context.name
        							servicePort: v
        						}
        					},
        				]
        			}
        		}]
        	}
        }
        
`

var traitDefWithKube = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: service-kube
  namespace: default
spec:
  appliesToWorkloads:
    - webservice
    - worker
    - backend
  podDisruptive: true
  schematic:
    kube:
      template:
        apiVersion: v1
        kind: Service
        metadata:
          name: my-service
        spec:
          ports:
            - protocol: TCP
              port: 80
              targetPort: 9376
      parameters:
        - name: targetPort
          required: true
          type: number
          fieldPaths:
            - "spec.template.spec.ports[0].targetPort"
          description: "target port num for service provider."
`

var componentWithDeepCue = `
# Test for deeper parameter in cue Template
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
 name: mywebservice
 namespace: default
 annotations:
   definition.oam.dev/description: "Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers."
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
       				if parameter.addRevisionLabel {
       					"app.oam.dev/appRevision": context.appRevision
       				}
       			}

       			spec: {
       				containers: [{
       					name:  context.name
       					image: parameter.image

       					if parameter["env"] != _|_ {
       						env: parameter.env
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

       	// +ignore
       	// +usage=If addRevisionLabel is true, the appRevision label will be added to the underlying pods
       	addRevisionLabel: *false | bool

       	// +usage=Define arguments by using environment variables
       	env?: [...{
       		// +usage=Environment variable name
       		name: string
       		// +usage=The value of the environment variable
       		value?: string
       		// +usage=Specifies a source the value of this var should come from
       		valueFrom?: {
       			// +usage=Selects a key of a secret in the pod's namespace
       			secretKeyRef: {
       				// +usage=The name of the secret in the pod's namespace to select from
       				name: string
                       // +ignore
       				// +usage=The key of the secret to select from. Must be a valid secret key
       				mySecretKey: string
       			}
       		}
       	}]
       }
`

var dryRunResult = `---
# Application(test-vela-app) -- Component(express-server) 
---

apiVersion: apps/v1
kind: Deployment
metadata:
  annotations: {}
  labels:
    app.oam.dev/appRevision: ""
    app.oam.dev/component: express-server
    app.oam.dev/name: test-vela-app
    app.oam.dev/namespace: default
    app.oam.dev/resourceType: WORKLOAD
    workload.oam.dev/type: test-webservice
  name: express-server
  namespace: default
spec:
  selector:
    matchLabels:
      app.oam.dev/component: express-server
  template:
    metadata:
      labels:
        app.oam.dev/component: express-server
    spec:
      containers:
      - image: crccheck/hello-world
        name: express-server
        ports:
        - containerPort: 80

---
## From the trait test-ingress 
apiVersion: v1
kind: Service
metadata:
  annotations: {}
  labels:
    app.oam.dev/appRevision: ""
    app.oam.dev/component: express-server
    app.oam.dev/name: test-vela-app
    app.oam.dev/namespace: default
    app.oam.dev/resourceType: TRAIT
    trait.oam.dev/resource: service
    trait.oam.dev/type: test-ingress
  name: express-server
  namespace: default
spec:
  ports:
  - port: 80
    targetPort: 80
  selector:
    app.oam.dev/component: express-server

---
## From the trait test-ingress 
apiVersion: networking.k8s.io/v1beta1
kind: Ingress
metadata:
  annotations: {}
  labels:
    app.oam.dev/appRevision: ""
    app.oam.dev/component: express-server
    app.oam.dev/name: test-vela-app
    app.oam.dev/namespace: default
    app.oam.dev/resourceType: TRAIT
    trait.oam.dev/resource: ingress
    trait.oam.dev/type: test-ingress
  name: express-server
  namespace: default
spec:
  rules:
  - host: testsvc.example.com
    http:
      paths:
      - backend:
          serviceName: express-server
          servicePort: 80
        path: /

---
`

var livediffResult = `Application (test-vela-app) has been modified(*)
  apiVersion: core.oam.dev/v1beta1
  kind: Application
  metadata:
    creationTimestamp: null
-   finalizers:
-   - app.oam.dev/resource-tracker-finalizer
    name: test-vela-app
    namespace: default
  spec:
    components:
-   - name: express-server
+   - name: new-express-server
      properties:
+       cpu: "0.5"
        image: crccheck/hello-world
-       port: 80
+       port: 5000
      traits:
      - properties:
-         domain: testsvc.example.com
+         domain: new-testsvc.example.com
          http:
-           /: 80
+           /: 8080
        type: test-ingress
      type: test-webservice
  status: {}
  
* Component (express-server) has been removed(-)
- apiVersion: apps/v1
- kind: Deployment
- metadata:
-   labels:
-     app.oam.dev/component: express-server
-     app.oam.dev/name: test-vela-app
-     app.oam.dev/namespace: default
-     app.oam.dev/resourceType: WORKLOAD
-     workload.oam.dev/type: test-webservice
-   name: express-server
-   namespace: default
- spec:
-   selector:
-     matchLabels:
-       app.oam.dev/component: express-server
-   template:
-     metadata:
-       labels:
-         app.oam.dev/component: express-server
-     spec:
-       containers:
-       - image: crccheck/hello-world
-         name: express-server
-         ports:
-         - containerPort: 80
  
* Component (express-server) / Trait (test-ingress/service) has been removed(-)
- apiVersion: v1
- kind: Service
- metadata:
-   labels:
-     app.oam.dev/component: express-server
-     app.oam.dev/name: test-vela-app
-     app.oam.dev/namespace: default
-     app.oam.dev/resourceType: TRAIT
-     trait.oam.dev/resource: service
-     trait.oam.dev/type: test-ingress
-   name: express-server
-   namespace: default
- spec:
-   ports:
-   - port: 80
-     targetPort: 80
-   selector:
-     app.oam.dev/component: express-server
  
* Component (express-server) / Trait (test-ingress/ingress) has been removed(-)
- apiVersion: networking.k8s.io/v1beta1
- kind: Ingress
- metadata:
-   labels:
-     app.oam.dev/component: express-server
-     app.oam.dev/name: test-vela-app
-     app.oam.dev/namespace: default
-     app.oam.dev/resourceType: TRAIT
-     trait.oam.dev/resource: ingress
-     trait.oam.dev/type: test-ingress
-   name: express-server
-   namespace: default
- spec:
-   rules:
-   - host: testsvc.example.com
-     http:
-       paths:
-       - backend:
-           serviceName: express-server
-           servicePort: 80
-         path: /
  
* Component (new-express-server) has been added(+)
+ apiVersion: apps/v1
+ kind: Deployment
+ metadata:
+   labels:
+     app.oam.dev/component: new-express-server
+     app.oam.dev/name: test-vela-app
+     app.oam.dev/namespace: default
+     app.oam.dev/resourceType: WORKLOAD
+     workload.oam.dev/type: test-webservice
+   name: new-express-server
+   namespace: default
+ spec:
+   selector:
+     matchLabels:
+       app.oam.dev/component: new-express-server
+   template:
+     metadata:
+       labels:
+         app.oam.dev/component: new-express-server
+     spec:
+       containers:
+       - image: crccheck/hello-world
+         name: new-express-server
+         ports:
+         - containerPort: 5000
+         resources:
+           limits:
+             cpu: "0.5"
+           requests:
+             cpu: "0.5"
  
* Component (new-express-server) / Trait (test-ingress/service) has been added(+)
+ apiVersion: v1
+ kind: Service
+ metadata:
+   labels:
+     app.oam.dev/component: new-express-server
+     app.oam.dev/name: test-vela-app
+     app.oam.dev/namespace: default
+     app.oam.dev/resourceType: TRAIT
+     trait.oam.dev/resource: service
+     trait.oam.dev/type: test-ingress
+   name: new-express-server
+   namespace: default
+ spec:
+   ports:
+   - port: 8080
+     targetPort: 8080
+   selector:
+     app.oam.dev/component: new-express-server
  
* Component (new-express-server) / Trait (test-ingress/ingress) has been added(+)
+ apiVersion: networking.k8s.io/v1beta1
+ kind: Ingress
+ metadata:
+   labels:
+     app.oam.dev/component: new-express-server
+     app.oam.dev/name: test-vela-app
+     app.oam.dev/namespace: default
+     app.oam.dev/resourceType: TRAIT
+     trait.oam.dev/resource: ingress
+     trait.oam.dev/type: test-ingress
+   name: new-express-server
+   namespace: default
+ spec:
+   rules:
+   - host: new-testsvc.example.com
+     http:
+       paths:
+       - backend:
+           serviceName: new-express-server
+           servicePort: 8080
+         path: /
`

var testShowComponentDef = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: test-show-task
  namespace: vela-system
spec:
  workload:
    definition:
      apiVersion: batch/v1
      kind: Job
  schematic:
    cue:
      template: |
        output: {
        	apiVersion: "batch/v1"
        	kind:       "Job"
        	spec: {
        		parallelism: parameter.count
        		completions: parameter.count
        		template: spec: {
        			restartPolicy: parameter.restart
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
        parameter: {
        	// +usage=specify number of tasks to run in parallel
        	// +short=c
        	count: *1 | int
        
        	// +usage=Which image would you like to use for your service
        	// +short=i
        	image: string
        
        	// +usage=Define the job restart policy, the value can only be Never or OnFailure. By default, it's Never.
        	restart: *"Never" | string
        
        	// +usage=Commands to run in the container
        	cmd?: [...string]
        }
`

var testShowTraitDef = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: test-sidecar
  namespace: vela-system
spec:
  appliesToWorkloads:
    - deployments.apps
  schematic:
    cue:
      template: |-
        patch: {
        	// +patchKey=name
        	spec: template: spec: containers: [parameter]
        }
        parameter: {
        	name:  string
        	image: string
        	command?: [...string]
        }
`

var showCdResult = `# Specification
+---------+--------------------------------------------------------------------------------------------------+----------+----------+---------+
|  NAME   |                                           DESCRIPTION                                            |   TYPE   | REQUIRED | DEFAULT |
+---------+--------------------------------------------------------------------------------------------------+----------+----------+---------+
| count   | specify number of tasks to run in parallel.                                                      | int      | false    |       1 |
| image   | Which image would you like to use for your service.                                              | string   | true     |         |
| restart | Define the job restart policy, the value can only be Never or OnFailure. By default, it's Never. | string   | false    | Never   |
| cmd     | Commands to run in the container.                                                                | []string | false    |         |
+---------+--------------------------------------------------------------------------------------------------+----------+----------+---------+


`

var showTdResult = `# Specification
+---------+-------------+----------+----------+---------+
|  NAME   | DESCRIPTION |   TYPE   | REQUIRED | DEFAULT |
+---------+-------------+----------+----------+---------+
| name    |             | string   | true     |         |
| image   |             | string   | true     |         |
| command |             | []string | false    |         |
+---------+-------------+----------+----------+---------+


`
