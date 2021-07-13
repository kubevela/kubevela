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

package controllers_test

import (
	"context"
	"fmt"
	"time"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kind/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// For legacy clusters that use appContext to create/update resources, we should guarantee backward compatibility while we
// deprecate appContext and replace it with assemble/dispatch modules.
// This test is to simulate a scenario where a cluster has an application whose resources are already created and owned
// by appContext and resource tracker(for cx-ns), and once we create a new application whose resources are same as
// existing ones, existing resources's ctrl-owner should be changed from appContext to resource tracker.
var _ = Describe("Test compatibility for deprecation of appContext", func() {
	ctx := context.Background()
	var namespaceName string
	var ns corev1.Namespace

	BeforeEach(func() {
		namespaceName = randomNamespaceName("deprecation-appctx-test")
		ns = corev1.Namespace{}
		ns.SetName(namespaceName)
		Expect(k8sClient.Create(ctx, &ns)).Should(Succeed())
		_, err := createFromYAML(ctx, pvTraitDefinition, namespaceName, nil)
		Expect(err).Should(BeNil())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &ns)).Should(Succeed())
		Expect(k8sClient.DeleteAllOf(ctx, &corev1.PersistentVolume{})).Should(Succeed())
	})

	It("Test application can update its resources' owners", func() {
		var err error
		var appCtxKey, rtKey *client.ObjectKey

		By("Mock existing owners in a legacy cluster")
		appCtxKey, err = createFromYAML(ctx, legacyAppCtx, namespaceName, nil)
		Expect(err).Should(BeNil())
		rtKey, err = createFromYAML(ctx, legacyResourceTracker, namespaceName, nil)
		Expect(err).Should(BeNil())
		By("Mock owner references: appCtx owns Deployment and Service")
		appCtx := &v1alpha2.ApplicationContext{}
		Expect(k8sClient.Get(ctx, *appCtxKey, appCtx)).Should(Succeed())
		appCtxOwnRef := meta.AsController(&v1alpha1.TypedReference{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "ApplicationContext",
			Name:       appCtx.GetName(),
			UID:        appCtx.GetUID(),
		})

		By("Mock owner references: rscTracker owns PersistentVolume")
		rt := &v1beta1.ResourceTracker{}
		Expect(k8sClient.Get(ctx, *rtKey, rt)).Should(Succeed())
		rtOwnerRef := meta.AsController(&v1alpha1.TypedReference{
			APIVersion: "core.oam.dev/v1beta1",
			Kind:       "ResourceTracker",
			Name:       rt.GetName(),
			UID:        rt.GetUID(),
		})

		var deployKey, svcKey, pvKey *client.ObjectKey
		By("Mock existing resources in a legacy cluster")
		deployKey, err = createFromYAML(ctx, legacyDeploy, namespaceName, &appCtxOwnRef)
		Expect(err).Should(BeNil())
		svcKey, err = createFromYAML(ctx, legacyService, namespaceName, &appCtxOwnRef)
		Expect(err).Should(BeNil())
		pvKey, err = createFromYAML(ctx, legacyPersistentVolume, namespaceName, &rtOwnerRef)
		Expect(err).Should(BeNil())
		By("Create an application")
		_, err = createFromYAML(ctx, newApplication, namespaceName, nil)
		Expect(err).Should(BeNil())

		By("Wait for new resource tracker is created")
		Eventually(func() error {
			rt = &v1beta1.ResourceTracker{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: "myapp-v1-" + namespaceName}, rt); err != nil {
				return errors.Wrap(err, "cannot get new resource tracker")
			}
			return nil
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())
		wantNewOwner := metav1.OwnerReference{
			APIVersion:         "core.oam.dev/v1beta1",
			Kind:               "ResourceTracker",
			Name:               rt.GetName(),
			UID:                rt.GetUID(),
			Controller:         pointer.BoolPtr(true),
			BlockOwnerDeletion: pointer.BoolPtr(true),
		}

		By("Verify existing resources' new owners")
		deploy := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, *deployKey, deploy)).Should(Succeed())
		newDeployOwner := metav1.GetControllerOf(deploy)
		Expect(newDeployOwner).ShouldNot(BeNil())
		Expect(*newDeployOwner).Should(BeEquivalentTo(wantNewOwner))

		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, *svcKey, svc)).Should(Succeed())
		newSvcOwner := metav1.GetControllerOf(svc)
		Expect(newSvcOwner).ShouldNot(BeNil())
		Expect(*newSvcOwner).Should(Equal(wantNewOwner))

		pv := &corev1.PersistentVolume{}
		Expect(k8sClient.Get(ctx, *pvKey, pv)).Should(Succeed())
		newPVOwner := metav1.GetControllerOf(svc)
		Expect(newPVOwner).ShouldNot(BeNil())
		Expect(*newPVOwner).Should(Equal(wantNewOwner))

		By("Delete the application")
		app := &v1beta1.Application{}
		app.SetName("myapp")
		app.SetNamespace(namespaceName)
		Expect(k8sClient.Delete(ctx, app)).Should(Succeed())

		By("Verify all resources can be deleted")
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: "myapp-v1-" + namespaceName}, rt)
		}, 10*time.Second, 500*time.Millisecond).Should(util.NotFoundMatcher{})
		Eventually(func() error {
			return k8sClient.Get(ctx, *deployKey, deploy)
		}, 30*time.Second, 500*time.Millisecond).Should(util.NotFoundMatcher{})
		Eventually(func() error {
			return k8sClient.Get(ctx, *svcKey, svc)
		}, 30*time.Second, 500*time.Millisecond).Should(util.NotFoundMatcher{})
		Eventually(func() error {
			return k8sClient.Get(ctx, *pvKey, pv)
		}, 30*time.Second, 500*time.Millisecond).Should(util.NotFoundMatcher{})
	})

	It("Test delete an application with a legacy finalizer", func() {
		var err error
		var rtKey *client.ObjectKey
		// simulate a resource tracker created by a legacy application
		rtKey, err = createFromYAML(ctx, legacyResourceTracker, namespaceName, nil)
		Expect(err).Should(BeNil())

		By("Create the application")
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "myapp",
				Namespace: namespaceName,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []v1beta1.ApplicationComponent{{
					Name:       "mycomp",
					Type:       "worker",
					Properties: runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
				}},
			},
		}
		app.SetFinalizers([]string{
			// this finalizer only apperars in a legacy application
			// this case is to test whether a legacy application with this finalizer can be deleted
			"resourceTracker.finalizer.core.oam.dev",
		})
		Expect(k8sClient.Create(ctx, app.DeepCopy())).Should(Succeed())

		By("Delete the application")
		Expect(k8sClient.Delete(ctx, app)).Should(Succeed())

		By("Verify legacy resource tracker is deleted")
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: rtKey.Name}, &v1beta1.ResourceTracker{})
		}, 10*time.Second, 500*time.Millisecond).Should(util.NotFoundMatcher{})
	})

})

var createFromYAML = func(ctx context.Context, objYAML string, ns string, owner *metav1.OwnerReference) (*client.ObjectKey, error) {
	u := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(fmt.Sprintf(objYAML, ns)), u); err != nil {
		return nil, err
	}
	if owner != nil {
		u.SetOwnerReferences([]metav1.OwnerReference{*owner})
	}
	objKey := client.ObjectKey{
		Name:      u.GetName(),
		Namespace: ns,
	}
	// use apply.Applicator to simulate real scenario
	applicator := apply.NewAPIApplicator(k8sClient)
	if err := applicator.Apply(ctx, u); err != nil {
		return nil, err
	}
	return &objKey, nil
}

var (
	legacyDeploy = `apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.oam.dev/app-revision-hash: 3dc894b5cb767c4b
    app.oam.dev/appRevision: myapp-v1
    app.oam.dev/component: myworker
    app.oam.dev/name: myapp
    app.oam.dev/resourceType: WORKLOAD
    app.oam.dev/revision: myworker-v1
    workload.oam.dev/type: worker
  name: myworker
  namespace: %s
spec:
  selector:
    matchLabels:
      app.oam.dev/component: myworker
  template:
    metadata:
      labels:
        app.oam.dev/component: myworker
    spec:
      containers:
      - image: nginx:latest
        name: myworker`

	legacyService = `apiVersion: v1
kind: Service
metadata:
  labels:
    app.oam.dev/app-revision-hash: 3dc894b5cb767c4b
    app.oam.dev/appRevision: myapp-v1
    app.oam.dev/component: myworker
    app.oam.dev/name: myapp
    app.oam.dev/resourceType: TRAIT
    app.oam.dev/revision: myworker-v1
    trait.oam.dev/resource: service
    trait.oam.dev/type: ingress
  name: myworker
  namespace: %s
spec:
  ports:
  - port: 8080
    protocol: TCP
    targetPort: 8080
  selector:
    app.oam.dev/component: myworker
  type: ClusterIP`

	legacyPersistentVolume = `apiVersion: v1
kind: PersistentVolume
metadata:
  labels:
    app.oam.dev/appRevision: myapp-v1
    app.oam.dev/component: myworker
    app.oam.dev/name: myapp
    app.oam.dev/resourceType: TRAIT
    app.oam.dev/revision: myworker-v1
    trait.oam.dev/resource: pv
    trait.oam.dev/type: testpv
  name: csi-gcs-pv-myworker
  namespace: %s
spec:
  accessModes:
  - ReadWriteOnce
  capacity:
    storage: 5Gi
  nfs:
    server: 1.1.1.1
    path: "/"
  persistentVolumeReclaimPolicy: Retain
  storageClassName: csi-gcs-test-sc`

	legacyAppCtx = `apiVersion: core.oam.dev/v1alpha2
kind: ApplicationContext
metadata:
  labels:
    app.oam.dev/app-revision-hash: 3dc894b5cb767c4b
  name: myapp
  namespace: %s
spec:
  applicationRevisionName: myapp-v1`

	legacyResourceTracker = `apiVersion: core.oam.dev/v1beta1
kind: ResourceTracker
metadata:
  name: %s-myapp`

	newApplication = `apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: myapp
  namespace: %s
spec:
  components:
    - name: myworker
      type: worker
      properties:
        image: "nginx:latest"
      traits:
        - type: ingress
          properties:
            domain: localhost 
            http:
              "/": 8080
        - type: testpv
          properties:
            secretName: testSecret`

	pvTraitDefinition = `apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: testpv
  namespace: %s
  annotations:
    definition.oam.dev/description: Mock a cluster-scope resource
spec:
  schematic:
    cue:
      template: |
        parameter: {
        	secretName: string
        }
        outputs: {
        	pv: {
        		apiVersion: "v1"
        		kind:       "PersistentVolume"
        		metadata: name: "csi-gcs-pv-\(context.name)"
        		spec: {
        			accessModes: ["ReadWriteOnce"]
        			capacity: storage: "5Gi"
        			persistentVolumeReclaimPolicy: "Retain"
        			storageClassName:              "csi-gcs-test-sc"
                    nfs: {
                        server: "1.1.1.1"
                        path: "/"
        			}
        		}
        	}
        }`
)
