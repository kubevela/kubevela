/*
Copyright 2021 The KubeVela Authors.

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

package resourcekeeper

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

var _ = Describe("Test ResourceKeeper StateKeep", func() {

	createConfigMapClusterObjectReference := func(name string) common.ClusterObjectReference {
		return common.ClusterObjectReference{
			ObjectReference: corev1.ObjectReference{
				Kind:       "ConfigMap",
				APIVersion: corev1.SchemeGroupVersion.String(),
				Name:       name,
				Namespace:  "default",
			},
		}
	}

	createConfigMapWithSharedBy := func(name string, ns string, appName string, sharedBy string, value string) *unstructured.Unstructured {
		o := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":      name,
					"namespace": ns,
					"labels": map[string]interface{}{
						oam.LabelAppName:      appName,
						oam.LabelAppNamespace: ns,
					},
					"annotations": map[string]interface{}{oam.AnnotationAppSharedBy: sharedBy},
				},
				"data": map[string]interface{}{
					"key": value,
				},
			},
		}
		o.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
		return o
	}

	createConfigMap := func(name string, value string) *unstructured.Unstructured {
		return createConfigMapWithSharedBy(name, "default", "", "", value)
	}

	It("Test StateKeep for various scene", func() {
		cli := testClient

		setOwner := func(obj *unstructured.Unstructured) {
			labels := obj.GetLabels()
			if labels == nil {
				labels = map[string]string{}
			}
			labels[oam.LabelAppName] = "app"
			labels[oam.LabelAppNamespace] = "default"
			obj.SetLabels(labels)
		}

		// state-keep add this resource
		cm1 := createConfigMap("cm1", "value")
		setOwner(cm1)
		cmRaw1, err := json.Marshal(cm1)
		Expect(err).Should(Succeed())

		// state-keep skip this resource
		cm2 := createConfigMap("cm2", "value")
		setOwner(cm2)
		Expect(cli.Create(context.Background(), cm2)).Should(Succeed())

		// state-keep delete this resource
		cm3 := createConfigMap("cm3", "value")
		setOwner(cm3)
		Expect(cli.Create(context.Background(), cm3)).Should(Succeed())

		// state-keep delete this resource
		cm4 := createConfigMap("cm4", "value")
		setOwner(cm4)
		cmRaw4, err := json.Marshal(cm4)
		Expect(err).Should(Succeed())
		Expect(cli.Create(context.Background(), cm4)).Should(Succeed())

		// state-keep update this resource
		cm5 := createConfigMap("cm5", "value")
		setOwner(cm5)
		cmRaw5, err := json.Marshal(cm5)
		Expect(err).Should(Succeed())
		cm5.Object["data"].(map[string]interface{})["key"] = "changed"
		Expect(cli.Create(context.Background(), cm5)).Should(Succeed())

		app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"}}
		h := &resourceKeeper{
			Client:     cli,
			app:        app,
			applicator: apply.NewAPIApplicator(cli),
			cache:      newResourceCache(cli, app),
		}

		h._currentRT = &v1beta1.ResourceTracker{
			Spec: v1beta1.ResourceTrackerSpec{
				ManagedResources: []v1beta1.ManagedResource{{
					ClusterObjectReference: createConfigMapClusterObjectReference("cm1"),
					Data:                   &runtime.RawExtension{Raw: cmRaw1},
				}, {
					ClusterObjectReference: createConfigMapClusterObjectReference("cm2"),
				}, {
					ClusterObjectReference: createConfigMapClusterObjectReference("cm3"),
					Deleted:                true,
				}, {
					ClusterObjectReference: createConfigMapClusterObjectReference("cm4"),
					Data:                   &runtime.RawExtension{Raw: cmRaw4},
					Deleted:                true,
				}, {
					ClusterObjectReference: createConfigMapClusterObjectReference("cm5"),
					Data:                   &runtime.RawExtension{Raw: cmRaw5},
				}},
			},
		}

		Expect(h.StateKeep(context.Background())).Should(Succeed())
		cms := &unstructured.UnstructuredList{}
		cms.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ConfigMap"))
		Expect(cli.List(context.Background(), cms, client.InNamespace("default"))).Should(Succeed())
		Expect(len(cms.Items)).Should(Equal(3))
		Expect(cms.Items[0].GetName()).Should(Equal("cm1"))
		Expect(cms.Items[1].GetName()).Should(Equal("cm2"))
		Expect(cms.Items[2].GetName()).Should(Equal("cm5"))
		Expect(cms.Items[2].Object["data"].(map[string]interface{})["key"].(string)).Should(Equal("value"))

		Expect(cli.Get(context.Background(), client.ObjectKeyFromObject(cm1), cm1)).Should(Succeed())
		cm1.SetLabels(map[string]string{
			oam.LabelAppName:      "app-2",
			oam.LabelAppNamespace: "default",
		})
		Expect(cli.Update(context.Background(), cm1)).Should(Succeed())
		err = h.StateKeep(context.Background())
		Expect(err).ShouldNot(Succeed())
		Expect(err.Error()).Should(ContainSubstring("failed to re-apply"))
	})

	It("Test StateKeep for shared resources", func() {
		cli := testClient
		ctx := context.Background()
		Expect(cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-shared"}})).Should(Succeed())
		cm1 := createConfigMapWithSharedBy("cm1", "test-shared", "app", "test-shared/app", "x")
		cmRaw1, err := json.Marshal(cm1)
		Expect(err).Should(Succeed())
		cm2 := createConfigMapWithSharedBy("cm2", "test-shared", "app", "", "y")
		cmRaw2, err := json.Marshal(cm2)
		Expect(err).Should(Succeed())
		app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "test-shared"}}
		h := &resourceKeeper{
			Client:     cli,
			app:        app,
			applicator: apply.NewAPIApplicator(cli),
			cache:      newResourceCache(cli, app),
		}
		h.sharedResourcePolicy = &v1alpha1.SharedResourcePolicySpec{Rules: []v1alpha1.SharedResourcePolicyRule{{
			Selector: v1alpha1.ResourcePolicyRuleSelector{ResourceTypes: []string{"ConfigMap"}},
		}}}
		h._currentRT = &v1beta1.ResourceTracker{
			Spec: v1beta1.ResourceTrackerSpec{
				ManagedResources: []v1beta1.ManagedResource{{
					ClusterObjectReference: createConfigMapClusterObjectReference("cm1"),
					Data:                   &runtime.RawExtension{Raw: cmRaw1},
				}, {
					ClusterObjectReference: createConfigMapClusterObjectReference("cm2"),
					Data:                   &runtime.RawExtension{Raw: cmRaw2},
				}},
			},
		}
		cm1 = createConfigMapWithSharedBy("cm1", "test-shared", "app", "test-shared/app,test-shared/another", "z")
		Expect(cli.Create(ctx, cm1)).Should(Succeed())
		cm2 = createConfigMapWithSharedBy("cm2", "test-shared", "another", "test-shared/another,test-shared/app", "z")
		Expect(cli.Create(ctx, cm2)).Should(Succeed())
		Expect(h.StateKeep(ctx)).Should(Succeed())
		Expect(cli.Get(ctx, client.ObjectKeyFromObject(cm1), cm1)).Should(Succeed())
		Expect(cm1.Object["data"].(map[string]interface{})["key"]).Should(Equal("x"))
		Expect(cli.Get(ctx, client.ObjectKeyFromObject(cm2), cm2)).Should(Succeed())
		Expect(cm2.Object["data"].(map[string]interface{})["key"]).Should(Equal("z"))
	})

	It("Test StateKeep for apply-once policy", func() {

		clusterManifest := &unstructured.Unstructured{}
		clusterJson, err := yaml.YAMLToJSON([]byte(clusterYaml))
		Expect(err).Should(Succeed())
		err = json.Unmarshal(clusterJson, clusterManifest)
		Expect(err).Should(Succeed())

		memoryManifest := &unstructured.Unstructured{}
		memoryJson, err := yaml.YAMLToJSON([]byte(memoryYaml))
		Expect(err).Should(Succeed())
		err = json.Unmarshal(memoryJson, memoryManifest)
		Expect(err).Should(Succeed())

		// state-keep skip spec.replicas
		pathWithReplicas := []string{"spec.replicas"}
		replicasValue, err := fieldpath.Pave(clusterManifest.UnstructuredContent()).GetValue(pathWithReplicas[0])
		Expect(err).Should(Succeed())
		err = fieldpath.Pave(memoryManifest.UnstructuredContent()).SetValue(pathWithReplicas[0], replicasValue)
		Expect(err).Should(Succeed())
		newReplicasValue, err := fieldpath.Pave(memoryManifest.UnstructuredContent()).GetValue(pathWithReplicas[0])
		Expect(err).Should(Succeed())
		Expect(reflect.DeepEqual(replicasValue, newReplicasValue)).Should(Equal(true))

		// state-keep skip spec.template.spec.containers[0].image
		pathWithImage := []string{"spec.template.spec.containers[0].image"}
		imageValue, err := fieldpath.Pave(clusterManifest.UnstructuredContent()).GetValue(pathWithImage[0])
		Expect(err).Should(Succeed())
		err = fieldpath.Pave(memoryManifest.UnstructuredContent()).SetValue(pathWithImage[0], imageValue)
		Expect(err).Should(Succeed())
		newImageValue, err := fieldpath.Pave(memoryManifest.UnstructuredContent()).GetValue(pathWithImage[0])
		Expect(err).Should(Succeed())
		Expect(reflect.DeepEqual(imageValue, newImageValue)).Should(Equal(true))

		// state-keep skip spec.template.spec.containers[0].resources
		pathWithResources := []string{"spec.template.spec.containers[0].resources"}
		resourcesValue, err := fieldpath.Pave(clusterManifest.UnstructuredContent()).GetValue(pathWithResources[0])
		Expect(err).Should(Succeed())
		err = fieldpath.Pave(memoryManifest.UnstructuredContent()).SetValue(pathWithResources[0], resourcesValue)
		Expect(err).Should(Succeed())
		newResourcesValue, err := fieldpath.Pave(memoryManifest.UnstructuredContent()).GetValue(pathWithResources[0])
		Expect(err).Should(Succeed())
		Expect(reflect.DeepEqual(resourcesValue, newResourcesValue)).Should(Equal(true))

		// state-keep with index error skip spec.template.spec.containers[1].resources
		pathWithIndexError := []string{"spec.template.spec.containers[1].resources"}
		_, err = fieldpath.Pave(clusterManifest.UnstructuredContent()).GetValue(pathWithIndexError[0])
		Expect(err).Should(Not(BeNil()))

		// state-keep with path error skip spec.template[0].spec.containers[0].resources
		pathWithPathError := []string{"spec.template[0].spec.containers[0].resources"}
		_, err = fieldpath.Pave(clusterManifest.UnstructuredContent()).GetValue(pathWithPathError[0])
		Expect(err).Should(Not(BeNil()))
	})

	It("Test StateKeep for FindStrategy", func() {

		cli := testClient
		createDeployment := func(name string, value *int32) *unstructured.Unstructured {
			o := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      name,
						"namespace": "default",
						"labels":    map[string]interface{}{oam.LabelAppComponent: name},
					},
					"spec": map[string]interface{}{
						"replicas": value,
					},
				},
			}
			o.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Deployment"))
			return o
		}

		// state-keep add this resource
		replicas := int32(2)
		deploy := createDeployment("fourierapp03-comp-01", &replicas)
		deployRaw, err := json.Marshal(deploy)
		Expect(err).Should(Succeed())

		createDeploymentClusterObjectReference := func(name string) common.ClusterObjectReference {
			return common.ClusterObjectReference{
				ObjectReference: corev1.ObjectReference{
					Kind:       "Deployment",
					APIVersion: corev1.SchemeGroupVersion.String(),
					Name:       name,
					Namespace:  "default",
				},
			}
		}

		app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "fourierapp03-comp-01",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\"}")},
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Name:       "apply-once-01",
						Type:       "apply-once",
						Properties: &runtime.RawExtension{Raw: []byte(`{"enable": true,"rules": [{"selector": { "componentNames": ["fourierapp03-comp-01"], "resourceTypes": ["Deployment" ], "strategy": {"path": ["spec.replicas"] } }}]}`)},
					},
				},
			}}
		h := &resourceKeeper{
			Client:     cli,
			app:        app,
			applicator: apply.NewAPIApplicator(cli),
			cache:      newResourceCache(cli, app),
			applyOncePolicy: &v1alpha1.ApplyOncePolicySpec{
				Enable: true,
				Rules: []v1alpha1.ApplyOncePolicyRule{{
					Selector: v1alpha1.ResourcePolicyRuleSelector{
						CompNames:     []string{"fourierapp03-comp-01"},
						ResourceTypes: []string{"Deployment"},
					},
					Strategy: &v1alpha1.ApplyOnceStrategy{Path: []string{"spec.replicas"}},
				},
				},
			},
		}
		h._currentRT = &v1beta1.ResourceTracker{
			Spec: v1beta1.ResourceTrackerSpec{
				ManagedResources: []v1beta1.ManagedResource{{
					ClusterObjectReference: createDeploymentClusterObjectReference("fourierapp03-comp-01"),
					Data:                   &runtime.RawExtension{Raw: deployRaw},
				}},
			},
		}
		applyOnceStrategy := h.applyOncePolicy.FindStrategy(deploy)
		Expect(applyOnceStrategy.Path).Should(Equal([]string{"spec.replicas"}))
	})
})

const (
	clusterYaml = `
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    app.io/display-name: fourier-container-040
    app.io/replicas: '1'
    deployment.kubernetes.io/revision: '31'
    io.cmb/liveness_probe_alert_level: warning
    io.cmb/readiness_probe_alert_level: warning
  creationTimestamp: '2022-01-12T05:59:50Z'
  generation: 77
  labels:
    app.io/name: fourier-container-040.lt31-04-fourier
    app.cmboam.io/name: fourier-appfile-040.lt31-04
    component.cmboam.io/name: fourier-component-040.lt31-04-fourier
    workload-type: Deployment
  name: fourier-container-040
  namespace: lt31-04-fourier
  resourceVersion: '547401259'
  uid: c74afeba-18a2-412a-84b4-bd48144356e0
spec:
  progressDeadlineSeconds: 600
  replicas: 10
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app.io/name: fourier-container-040.lt31-04-fourier
      workload-type: Deployment
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      creationTimestamp: null
      labels:
        workload-type: Deployment
    spec:
      affinity: {}
      containers:
        - image: 'cmb.cn/console/proj_gin_test:v2_clusterYaml'
          imagePullPolicy: IfNotPresent
          lifecycle:
            preStop:
              exec:
                command:
                  - sh
                  - '-c'
                  - sleep 30
          livenessProbe:
            failureThreshold: 3
            initialDelaySeconds: 30
            periodSeconds: 30
            successThreshold: 1
            tcpSocket:
              port: 8001
            timeoutSeconds: 5
          name: fourier-container-040
          ports:
            - containerPort: 8001
              protocol: TCP
          readinessProbe:
            failureThreshold: 3
            initialDelaySeconds: 30
            periodSeconds: 30
            successThreshold: 1
            tcpSocket:
              port: 8001
            timeoutSeconds: 5
          resources:
            limits:
              cpu: '10'
              memory: 10Gi
            requests:
              cpu: 10m
              memory: 10Mi
          terminationMessagePath: /dev/termination-log
          terminationMessagePolicy: File
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 30
status:
  availableReplicas: 1
  conditions:
    - lastTransitionTime: '2022-04-15T02:03:07Z'
      lastUpdateTime: '2022-04-15T02:03:07Z'
      message: Deployment has minimum availability.
      reason: MinimumReplicasAvailable
      status: 'True'
      type: Available
    - lastTransitionTime: '2022-01-12T07:00:52Z'
      lastUpdateTime: '2022-04-26T07:29:24Z'
      message: >-
        ReplicaSet "fourier-container-040-79b8f79fd9" has successfully
        progressed.
      reason: NewReplicaSetAvailable
      status: 'True'
      type: Progressing
  observedGeneration: 77
  readyReplicas: 1
  replicas: 1
  updatedReplicas: 1

`

	memoryYaml = `
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    app.io/display-name: fourier-container-040
    app.io/replicas: '1'
    deployment.kubernetes.io/revision: '31'
    io.cmb/liveness_probe_alert_level: warning
    io.cmb/readiness_probe_alert_level: warning
  creationTimestamp: '2022-01-12T05:59:50Z'
  generation: 77
  labels:
    app.io/name: fourier-container-040.lt31-04-fourier
    app.cmboam.io/name: fourier-appfile-040.lt31-04
    component.cmboam.io/name: fourier-component-040.lt31-04-fourier
    workload-type: Deployment
  name: fourier-container-040
  namespace: lt31-04-fourier
  resourceVersion: '547401259'
  uid: c74afeba-18a2-412a-84b4-bd48144356e0
spec:
  progressDeadlineSeconds: 600
  replicas: 5
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app.io/name: fourier-container-040.lt31-04-fourier
      workload-type: Deployment
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      creationTimestamp: null
      labels:
        workload-type: Deployment
    spec:
      affinity: {}
      containers:
        - image: 'cmb.cn/console/proj_gin_test:v2_memoryYaml'
          imagePullPolicy: IfNotPresent
          lifecycle:
            preStop:
              exec:
                command:
                  - sh
                  - '-c'
                  - sleep 30
          livenessProbe:
            failureThreshold: 3
            initialDelaySeconds: 30
            periodSeconds: 30
            successThreshold: 1
            tcpSocket:
              port: 8001
            timeoutSeconds: 5
          name: fourier-container-040
          ports:
            - containerPort: 8001
              protocol: TCP
          readinessProbe:
            failureThreshold: 3
            initialDelaySeconds: 30
            periodSeconds: 30
            successThreshold: 1
            tcpSocket:
              port: 8001
            timeoutSeconds: 5
          resources:
            limits:
              cpu: '5'
              memory: 5Gi
            requests:
              cpu: 5m
              memory: 5Mi
          terminationMessagePath: /dev/termination-log
          terminationMessagePolicy: File
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 30
status:
  availableReplicas: 1
  conditions:
    - lastTransitionTime: '2022-04-15T02:03:07Z'
      lastUpdateTime: '2022-04-15T02:03:07Z'
      message: Deployment has minimum availability.
      reason: MinimumReplicasAvailable
      status: 'True'
      type: Available
    - lastTransitionTime: '2022-01-12T07:00:52Z'
      lastUpdateTime: '2022-04-26T07:29:24Z'
      message: >-
        ReplicaSet "fourier-container-040-79b8f79fd9" has successfully
        progressed.
      reason: NewReplicaSetAvailable
      status: 'True'
      type: Progressing
  observedGeneration: 77
  readyReplicas: 1
  replicas: 1
  updatedReplicas: 1

`
)
