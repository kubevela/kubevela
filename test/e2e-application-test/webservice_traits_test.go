/*
Copyright 2026 The KubeVela Authors.

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

package application_test

import (
	"context"
	"strings"

	"github.com/guidewire-oss/sawchain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// traitCase describes one webservice Application with traits applied.
// The case name doubles as the Application, component, and Deployment name.
type traitCase struct {
	name string
	// traits is a YAML list injected into the component via (parse_yaml($traits)).
	traits string
	// expected is a multi-document Chainsaw template of resources that must exist.
	expected string
	// fixtures is an optional multi-document template of resources created before the Application.
	fixtures string
}

const appHealthy = `
	apiVersion: core.oam.dev/v1beta1
	kind: Application
	metadata:
	  name: ($name)
	  namespace: ($namespace)
	status:
	  status: running
	  services:
	  - name: ($name)
	    healthy: true
`

// dedent trims leading tabs from every line. Sawchain de-indents templates but not binding
// values, and YAML rejects tab indentation in parse_yaml. Nested YAML indentation uses spaces.
func dedent(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimLeft(l, "\t")
	}
	return strings.Join(lines, "\n")
}

var traitCases = []traitCase{
	{
		name: "scaler",
		traits: `
			- type: scaler
			  properties:
			    replicas: 2
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  replicas: 2
		`,
	},
	{
		name: "labels",
		traits: `
			- type: labels
			  properties:
			    tier: web
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			  labels:
			    tier: web
			spec:
			  template:
			    metadata:
			      labels:
			        tier: web
		`,
	},
	{
		name: "annotations",
		traits: `
			- type: annotations
			  properties:
			    team: platform
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			  annotations:
			    team: platform
			spec:
			  template:
			    metadata:
			      annotations:
			        team: platform
		`,
	},
	{
		name: "env",
		traits: `
			- type: env
			  properties:
			    env:
			      FOO: bar
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      containers:
			      - name: ($name)
			        env:
			        - name: FOO
			          value: bar
		`,
	},
	{
		name: "resource",
		traits: `
			- type: resource
			  properties:
			    cpu: 100m
			    memory: 64Mi
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      containers:
			      - name: ($name)
			        resources:
			          requests:
			            cpu: 100m
			            memory: 64Mi
			          limits:
			            cpu: 100m
			            memory: 64Mi
		`,
	},
	{
		name: "sidecar",
		traits: `
			- type: sidecar
			  properties:
			    name: helper
			    image: busybox:1.34
			    cmd: ["sleep", "86400"]
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      containers:
			      - name: ($name)
			      - name: helper
			        image: busybox:1.34
			        command: ["sleep", "86400"]
		`,
	},
	{
		name: "lifecycle",
		traits: `
			- type: lifecycle
			  properties:
			    preStop:
			      exec:
			        command: ["sleep", "1"]
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      containers:
			      - name: ($name)
			        lifecycle:
			          preStop:
			            exec:
			              command: ["sleep", "1"]
		`,
	},
	{
		name: "container-ports",
		traits: `
			- type: container-ports
			  properties:
			    ports:
			    - containerPort: 9090
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      containers:
			      - name: ($name)
			        ports:
			        - containerPort: 8080
			        - containerPort: 9090
			          protocol: TCP
		`,
	},
	{
		name: "hostalias",
		traits: `
			- type: hostalias
			  properties:
			    hostAliases:
			    - ip: 10.1.2.3
			      hostnames: [a.example.com]
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      hostAliases:
			      - ip: 10.1.2.3
			        hostnames: [a.example.com]
		`,
	},
	{
		name: "k8s-update-strategy",
		traits: `
			- type: k8s-update-strategy
			  properties:
			    strategy:
			      type: Recreate
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  strategy:
			    type: Recreate
		`,
	},
	{
		// The labels trait runs first so json-patch has a label to remove. The kept label proves the trait applied, so the removed label existed.
		name: "json-patch",
		traits: `
			- type: labels
			  properties:
			    to-delete: "yes"
			    keep: "yes"
			- type: json-patch
			  properties:
			    operations:
			    - op: add
			      path: /spec/replicas
			      value: 3
			    - op: add
			      path: /spec/template/metadata/labels/patched
			      value: "yes"
			    - op: remove
			      path: /spec/template/metadata/labels/to-delete
			    - op: add
			      path: /spec/template/spec/containers/1
			      value:
			        name: helper
			        image: busybox:1.34
			        command: ["sleep", "86400"]
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  replicas: 3
			  template:
			    metadata:
			      labels:
			        patched: "yes"
			        keep: "yes"
			        ("to-delete" == null): true
			    spec:
			      containers:
			      - name: ($name)
			      - name: helper
			        image: busybox:1.34
		`,
	},
	{
		name: "json-merge-patch",
		traits: `
			- type: labels
			  properties:
			    to-delete: "yes"
			    keep: "yes"
			- type: json-merge-patch
			  properties:
			    metadata:
			      labels:
			        deploy-label: added
			    spec:
			      replicas: 3
			      template:
			        metadata:
			          labels:
			            to-delete: null
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			  labels:
			    deploy-label: added
			spec:
			  replicas: 3
			  template:
			    metadata:
			      labels:
			        keep: "yes"
			        ("to-delete" == null): true
		`,
	},
	{
		// Auxiliary resource: full Service spec, as in Sawchain's gateway example.
		name: "expose",
		traits: `
			- type: expose
			  properties:
			    ports:
			    - port: 8080
		`,
		expected: `
			apiVersion: v1
			kind: Service
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  type: ClusterIP
			  selector:
			    app.oam.dev/component: ($name)
			  ports:
			  - name: port-8080
			    port: 8080
			    targetPort: 8080
			    protocol: TCP
		`,
	},
	{
		name: "command",
		traits: `
			- type: sidecar
			  properties:
			    name: helper
			    image: busybox:1.34
			    cmd: ["sleep", "1"]
			- type: command
			  properties:
			    containerName: helper
			    command: ["sleep"]
			    args: ["86400"]
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      containers:
			      - name: ($name)
			      - name: helper
			        command: ["sleep"]
			        args: ["86400"]
		`,
	},
	{
		name: "startup-probe",
		traits: `
			- type: startup-probe
			  properties:
			    tcpSocket:
			      port: 8080
			    failureThreshold: 10
			    periodSeconds: 5
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      containers:
			      - name: ($name)
			        startupProbe:
			          tcpSocket:
			            port: 8080
			          failureThreshold: 10
			          periodSeconds: 5
		`,
	},
	{
		name: "securitycontext",
		traits: `
			- type: securitycontext
			  properties:
			    runAsNonRoot: false
			    allowPrivilegeEscalation: false
			    dropCapabilities: [NET_RAW]
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      containers:
			      - name: ($name)
			        securityContext:
			          runAsNonRoot: false
			          allowPrivilegeEscalation: false
			          capabilities:
			            drop: [NET_RAW]
		`,
	},
	{
		name: "podsecuritycontext",
		traits: `
			- type: podsecuritycontext
			  properties:
			    runAsNonRoot: false
			    fsGroup: 1000
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      securityContext:
			        runAsNonRoot: false
			        fsGroup: 1000
		`,
	},
	{
		// ScheduleAnyway so the pod schedules on a single-node test cluster.
		name: "topologyspreadconstraints",
		traits: `
			- type: topologyspreadconstraints
			  properties:
			    constraints:
			    - maxSkew: 1
			      topologyKey: kubernetes.io/hostname
			      whenUnsatisfiable: ScheduleAnyway
			      labelSelector:
			        matchLabels:
			          app.oam.dev/component: topologyspreadconstraints
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      topologySpreadConstraints:
			      - maxSkew: 1
			        topologyKey: kubernetes.io/hostname
			        whenUnsatisfiable: ScheduleAnyway
			        labelSelector:
			          matchLabels:
			            app.oam.dev/component: topologyspreadconstraints
		`,
	},
	{
		// Same image as the base so the pod stays healthy. Always differs from the IfNotPresent default of a tagged image, so the override is observable. The image field is not discriminating.
		name: "container-image",
		traits: `
			- type: container-image
			  properties:
			    image: oamdev/testapp:v1
			    imagePullPolicy: Always
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      containers:
			      - name: ($name)
			        image: oamdev/testapp:v1
			        imagePullPolicy: Always
		`,
	},
	{
		name: "init-container",
		traits: `
			- type: init-container
			  properties:
			    name: init
			    image: busybox:1.34
			    cmd: ["sh", "-c", "echo ready > /init/ready"]
			    appMountPath: /app-shared
			    initMountPath: /init
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      initContainers:
			      - name: init
			        image: busybox:1.34
			        command: ["sh", "-c", "echo ready > /init/ready"]
			        volumeMounts:
			        - name: workdir
			          mountPath: /init
			      containers:
			      - name: ($name)
			        volumeMounts:
			        - name: workdir
			          mountPath: /app-shared
			      volumes:
			      - name: workdir
			        emptyDir: {}
		`,
	},
	{
		// Tolerations only: a nodeSelector or required affinity could leave the pod unschedulable.
		name: "affinity",
		traits: `
			- type: affinity
			  properties:
			    tolerations:
			    - key: example.com/test
			      operator: Exists
			      effect: NoSchedule
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      tolerations:
			      - key: example.com/test
			        operator: Exists
			        effect: NoSchedule
		`,
	},
	{
		// create=false: the ServiceAccount must already exist, so a fixture provides it.
		name: "service-account",
		traits: `
			- type: service-account
			  properties:
			    name: custom-sa
		`,
		fixtures: `
			apiVersion: v1
			kind: ServiceAccount
			metadata:
			  name: custom-sa
			  namespace: ($namespace)
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      serviceAccountName: custom-sa
		`,
	},
	{
		// Namespace-scoped privileges only; cluster scope would leak ClusterRoles past namespace cleanup.
		name: "service-account-create",
		traits: `
			- type: service-account
			  properties:
			    name: created-sa
			    create: true
			    privileges:
			    - verbs: [get, list]
			      apiGroups: [""]
			      resources: [configmaps]
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      serviceAccountName: created-sa
			---
			apiVersion: v1
			kind: ServiceAccount
			metadata:
			  name: created-sa
			  namespace: ($namespace)
			---
			apiVersion: rbac.authorization.k8s.io/v1
			kind: Role
			metadata:
			  name: created-sa
			  namespace: ($namespace)
			rules:
			- verbs: [get, list]
			  apiGroups: [""]
			  resources: [configmaps]
			---
			apiVersion: rbac.authorization.k8s.io/v1
			kind: RoleBinding
			metadata:
			  name: created-sa
			  namespace: ($namespace)
			roleRef:
			  apiGroup: rbac.authorization.k8s.io
			  kind: Role
			  name: created-sa
			subjects:
			- kind: ServiceAccount
			  name: created-sa
		`,
	},
	{
		// The trait creates the ConfigMap itself (mountOnly defaults to false).
		name: "storage",
		traits: `
			- type: storage
			  properties:
			    configMap:
			    - name: storage-cm
			      mountPath: /etc/cm
			      data:
			        key: value
			    emptyDir:
			    - name: scratch
			      mountPath: /scratch
		`,
		expected: `
			apiVersion: apps/v1
			kind: Deployment
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  template:
			    spec:
			      containers:
			      - name: ($name)
			        volumeMounts:
			        - name: configmap-storage-cm
			          mountPath: /etc/cm
			        - name: emptydir-scratch
			          mountPath: /scratch
			      volumes:
			      - name: configmap-storage-cm
			        configMap:
			          name: storage-cm
			      - name: emptydir-scratch
			        emptyDir: {}
			---
			apiVersion: v1
			kind: ConfigMap
			metadata:
			  name: storage-cm
			  namespace: ($namespace)
			data:
			  key: value
		`,
	},
	{
		// The Ingress needs no controller to be created. The trait also creates a same-named Service.
		name: "gateway",
		traits: `
			- type: gateway
			  properties:
			    domain: gateway.example.com
			    class: test-class
			    pathType: Prefix
			    http:
			      /: 8080
		`,
		expected: `
			apiVersion: v1
			kind: Service
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  selector:
			    app.oam.dev/component: ($name)
			  ports:
			  - name: port-8080
			    port: 8080
			    targetPort: 8080
			---
			apiVersion: networking.k8s.io/v1
			kind: Ingress
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			  annotations:
			    kubernetes.io/ingress.class: test-class
			spec:
			  rules:
			  - host: gateway.example.com
			    http:
			      paths:
			      - path: /
			        pathType: Prefix
			        backend:
			          service:
			            name: ($name)
			            port:
			              number: 8080
		`,
	},
	{
		// The HPA object is created without metrics-server; only scaling needs it.
		name: "hpa",
		traits: `
			- type: hpa
			  properties:
			    min: 2
			    max: 3
			    cpu:
			      type: Utilization
			      value: 60
		`,
		expected: `
			apiVersion: autoscaling/v2
			kind: HorizontalPodAutoscaler
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  scaleTargetRef:
			    apiVersion: apps/v1
			    kind: Deployment
			    name: ($name)
			  minReplicas: 2
			  maxReplicas: 3
			  metrics:
			  - type: Resource
			    resource:
			      name: cpu
			      target:
			        type: Utilization
			        averageUtilization: 60
		`,
	},
	{
		name: "cpuscaler",
		traits: `
			- type: cpuscaler
			  properties:
			    min: 2
			    max: 3
			    cpuUtil: 70
		`,
		expected: `
			apiVersion: autoscaling/v1
			kind: HorizontalPodAutoscaler
			metadata:
			  name: ($name)
			  namespace: ($namespace)
			spec:
			  scaleTargetRef:
			    apiVersion: apps/v1
			    kind: Deployment
			    name: ($name)
			  minReplicas: 2
			  maxReplicas: 3
			  targetCPUUtilizationPercentage: 70
		`,
	},
}

var _ = Describe("Webservice component traits (Sawchain)", Label("core-application"), func() {
	var (
		ctx       context.Context
		sc        *sawchain.Sawchain
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = randomNamespaceName("webservice-traits")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())
		sc = sawchain.New(GinkgoTB(), k8sClient, sawchainTimeout, sawchainInterval, map[string]any{"namespace": namespace})
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())
	})

	for _, c := range traitCases {
		It("applies the "+c.name+" trait", func() {
			bindings := map[string]any{"name": c.name, "traits": dedent(c.traits)}
			if c.fixtures != "" {
				sc.CreateAndWait(ctx, c.fixtures, bindings)
			}
			sc.CreateAndWait(ctx, "testdata/webservice/traits.tpl.yaml", bindings)

			Eventually(sc.CheckFunc(ctx, appHealthy, bindings), sawchainTimeout, sawchainInterval).Should(Succeed())
			Eventually(sc.CheckFunc(ctx, c.expected, bindings), sawchainTimeout, sawchainInterval).Should(Succeed())
		})
	}
})
