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

package addon

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	types2 "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	v1alpha12 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

var _ = Describe("Addon test", func() {
	ctx := context.Background()
	var app v1beta1.Application

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &app)).Should(BeNil())
	})

	It("continueOrRestartWorkflow func test", func() {
		app = v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(appYaml), &app)).Should(BeNil())
		app.SetNamespace(testns)
		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())

		Eventually(func() error {
			checkApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, types2.NamespacedName{Namespace: app.Namespace, Name: app.Name}, checkApp); err != nil {
				return err
			}
			appPatch := client.MergeFrom(checkApp.DeepCopy())
			checkApp.Status.Workflow = &common.WorkflowStatus{Suspend: true}
			if err := k8sClient.Status().Patch(ctx, checkApp, appPatch); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())

		Eventually(func() error {
			checkApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, types2.NamespacedName{Namespace: app.Namespace, Name: app.Name}, checkApp); err != nil {
				return err
			}
			if !checkApp.Status.Workflow.Suspend {
				return fmt.Errorf("app haven't not suspend")
			}

			h := Installer{ctx: ctx, cli: k8sClient, addon: &InstallPackage{Meta: Meta{Name: "test-app"}}}
			if err := h.continueOrRestartWorkflow(); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())

		Eventually(func() error {
			checkApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, types2.NamespacedName{Namespace: app.Namespace, Name: app.Name}, checkApp); err != nil {
				return err
			}
			if checkApp.Status.Workflow.Suspend {
				return fmt.Errorf("app haven't not continue")
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
	})

	It("continueOrRestartWorkflow func test, test restart workflow", func() {
		app = v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(appYaml), &app)).Should(BeNil())
		app.SetNamespace(testns)
		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())

		Eventually(func() error {
			checkApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, types2.NamespacedName{Namespace: app.Namespace, Name: app.Name}, checkApp); err != nil {
				return err
			}
			appPatch := client.MergeFrom(checkApp.DeepCopy())
			checkApp.Status.Workflow = &common.WorkflowStatus{Message: "someMessage", AppRevision: "test-revision"}
			checkApp.Status.Phase = common.ApplicationRunning
			if err := k8sClient.Status().Patch(ctx, checkApp, appPatch); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())

		Eventually(func() error {
			checkApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, types2.NamespacedName{Namespace: app.Namespace, Name: app.Name}, checkApp); err != nil {
				return err
			}
			if checkApp.Status.Phase != common.ApplicationRunning {
				return fmt.Errorf("app haven't not running")
			}

			h := Installer{ctx: ctx, cli: k8sClient, addon: &InstallPackage{Meta: Meta{Name: "test-app"}}}
			if err := h.continueOrRestartWorkflow(); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())

		Eventually(func() error {
			checkApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, types2.NamespacedName{Namespace: app.Namespace, Name: app.Name}, checkApp); err != nil {
				return err
			}
			if checkApp.Status.Workflow != nil {
				return fmt.Errorf("app workflow havenot been restart")
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
	})

	It(" FetchAddonRelatedApp func test", func() {
		app = v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(legacyAppYaml), &app)).Should(BeNil())
		app.SetNamespace(testns)
		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())

		Eventually(func() error {
			app, err := FetchAddonRelatedApp(ctx, k8sClient, "legacy-addon")
			if err != nil {
				return err
			}
			if app.Name != "legacy-addon" {
				return fmt.Errorf("error addon app name")
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
	})

	It(" determineAddonAppName func test", func() {
		app = v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(legacyAppYaml), &app)).Should(BeNil())
		app.SetNamespace(testns)
		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())

		Eventually(func() error {
			appName, err := determineAddonAppName(ctx, k8sClient, "legacy-addon")
			if err != nil {
				return err
			}
			if appName != "legacy-addon" {
				return fmt.Errorf("error addon app name")
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())

		notExsitAppName, err := determineAddonAppName(ctx, k8sClient, "not-exist")
		Expect(err).Should(BeNil())
		Expect(notExsitAppName).Should(BeEquivalentTo("addon-not-exist"))
	})
})

var _ = Describe("Addon func test", func() {
	var deploy appsv1.Deployment

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &deploy))
	})

	It("fetchVelaCoreImageTag func test", func() {
		deploy = appsv1.Deployment{}
		tag, err := fetchVelaCoreImageTag(ctx, k8sClient)
		Expect(err).ShouldNot(BeNil())
		Expect(tag).Should(BeEquivalentTo(""))

		Expect(yaml.Unmarshal([]byte(deployYaml), &deploy)).Should(BeNil())
		deploy.SetNamespace(types.DefaultKubeVelaNS)
		Expect(k8sClient.Create(ctx, &deploy)).Should(BeNil())

		Eventually(func() error {
			tag, err := fetchVelaCoreImageTag(ctx, k8sClient)
			if err != nil {
				return err
			}
			if tag != "v1.2.3" {
				return fmt.Errorf("tag missmatch want %s actual %s", "v1.2.3", tag)
			}
			return err
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
	})

	It("checkAddonVersionMeetRequired func test", func() {
		deploy = appsv1.Deployment{}
		Expect(checkAddonVersionMeetRequired(ctx, &SystemRequirements{VelaVersion: ">=v1.2.1"}, k8sClient, dc)).ShouldNot(BeNil())
		Expect(yaml.Unmarshal([]byte(deployYaml), &deploy)).Should(BeNil())
		deploy.SetNamespace(types.DefaultKubeVelaNS)
		Expect(k8sClient.Create(ctx, &deploy)).Should(BeNil())

		Expect(checkAddonVersionMeetRequired(ctx, &SystemRequirements{VelaVersion: ">=v1.2.1"}, k8sClient, dc)).Should(BeNil())
		Expect(checkAddonVersionMeetRequired(ctx, &SystemRequirements{VelaVersion: ">=v1.2.4"}, k8sClient, dc)).ShouldNot(BeNil())
	})
})

var _ = Describe("Test addon util func", func() {

	It("test render and fetch args", func() {
		i := InstallPackage{Meta: Meta{Name: "test-addon"}}
		args := map[string]interface{}{
			"imagePullSecrets": []string{
				"myreg", "myreg1",
			},
		}
		u := RenderArgsSecret(&i, args)
		secName := u.GetName()
		secNs := u.GetNamespace()
		Expect(k8sClient.Create(ctx, u)).Should(BeNil())

		sec := v1.Secret{}
		Expect(k8sClient.Get(ctx, types2.NamespacedName{Namespace: secNs, Name: secName}, &sec)).Should(BeNil())
		res, err := FetchArgsFromSecret(&sec)
		Expect(err).Should(BeNil())
		Expect(res).Should(BeEquivalentTo(map[string]interface{}{"imagePullSecrets": []interface{}{"myreg", "myreg1"}}))
	})

	It("test render and fetch args backward compatibility", func() {
		secArgs := v1.Secret{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      addonutil.Addon2SecName("test-addon-old-args"),
				Namespace: types.DefaultKubeVelaNS,
			},
			StringData: map[string]string{
				"repo": "www.test.com",
				"tag":  "v1.3.1",
			},
			Type: v1.SecretTypeOpaque,
		}
		secName := secArgs.GetName()
		secNs := secArgs.GetNamespace()
		Expect(k8sClient.Create(ctx, &secArgs)).Should(BeNil())

		sec := v1.Secret{}
		Expect(k8sClient.Get(ctx, types2.NamespacedName{Namespace: secNs, Name: secName}, &sec)).Should(BeNil())
		res, err := FetchArgsFromSecret(&sec)
		Expect(err).Should(BeNil())
		Expect(res).Should(BeEquivalentTo(map[string]interface{}{"repo": "www.test.com", "tag": "v1.3.1"}))
	})

})

var _ = Describe("Test render addon with specified clusters", func() {
	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "add-c1",
				Namespace: "vela-system",
				Labels: map[string]string{
					clustercommon.LabelKeyClusterCredentialType: string(v1alpha1.CredentialTypeX509Certificate),
					clustercommon.LabelKeyClusterEndpointType:   string(v1alpha1.ClusterEndpointTypeConst),
					"key": "value",
				},
			},
		})).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "add-c2",
				Namespace: "vela-system",
				Labels: map[string]string{
					clustercommon.LabelKeyClusterCredentialType: string(v1alpha1.CredentialTypeX509Certificate),
					clustercommon.LabelKeyClusterEndpointType:   string(v1alpha1.ClusterEndpointTypeConst),
					"key": "value",
				},
			},
		})).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})
	It("test render not exits cluster", func() {
		i := &baseAddon
		i.Name = "test-cluster-addon"

		args := map[string]interface{}{
			"clusters": []string{"add-c1", "ne"},
		}
		_, err := RenderApp(ctx, i, k8sClient, args)
		Expect(err.Error()).Should(BeEquivalentTo("cluster ne not exist"))
	})
	It("test render normal addon with specified clusters", func() {
		i := &baseAddon
		i.DeployTo = &DeployTo{RuntimeCluster: true}
		i.Name = "test-cluster-addon-normal"
		args := map[string]interface{}{
			"clusters": []string{"add-c1", "add-c2"},
		}
		ap, err := RenderApp(ctx, i, k8sClient, args)
		Expect(err).Should(BeNil())
		Expect(ap.Spec.Policies).Should(BeEquivalentTo([]v1beta1.AppPolicy{{Name: "specified-addon-clusters",
			Type:       v1alpha12.TopologyPolicyType,
			Properties: &runtime.RawExtension{Raw: []byte(`{"clusters":["add-c1","add-c2","local"]}`)}}}))
	})
})

var _ = Describe("func addon update ", func() {
	It("test update addon app label", func() {
		app_test_update := v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(addonUpdateAppYaml), &app_test_update)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &app_test_update)).Should(BeNil())

		Eventually(func() error {
			var err error
			appCheck := v1beta1.Application{}
			err = k8sClient.Get(ctx, types2.NamespacedName{Namespace: "vela-system", Name: "addon-test-update"}, &appCheck)
			if err != nil {
				return err
			}
			if appCheck.Labels["addons.oam.dev/version"] != "v1.2.0" {
				return fmt.Errorf("label missmatch")
			}
			return nil
		}, time.Millisecond*500, 30*time.Second).Should(BeNil())

		pkg := &InstallPackage{Meta: Meta{Name: "test-update", Version: "1.3.0"}}
		h := NewAddonInstaller(context.Background(), k8sClient, nil, nil, nil, &Registry{Name: "test"}, nil, nil)
		h.addon = pkg
		Expect(h.dispatchAddonResource(pkg)).Should(BeNil())

		Eventually(func() error {
			var err error
			appCheck := v1beta1.Application{}
			err = k8sClient.Get(context.Background(), types2.NamespacedName{Namespace: "vela-system", Name: "addon-test-update"}, &appCheck)
			if err != nil {
				return err
			}
			if appCheck.Labels["addons.oam.dev/version"] != "1.3.0" {
				return fmt.Errorf("label missmatch")
			}
			return nil
		}, time.Second*3, 300*time.Second).Should(BeNil())
	})
})

var _ = Describe("test enable addon in local dir", func() {
	BeforeEach(func() {
		app := v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "vela-system", Name: "addon-example"}}
		Expect(k8sClient.Delete(ctx, &app)).Should(SatisfyAny(BeNil(), util.NotFoundMatcher{}))
	})

	It("test enable addon by local dir", func() {
		ctx := context.Background()
		err := EnableAddonByLocalDir(ctx, "example", "./testdata/example", k8sClient, dc, apply.NewAPIApplicator(k8sClient), cfg, map[string]interface{}{"example": "test"})
		Expect(err).Should(BeNil())
		app := v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types2.NamespacedName{Namespace: "vela-system", Name: "addon-example"}, &app)).Should(BeNil())
	})
})

var _ = Describe("test enable addon which applies the views independently", func() {
	BeforeEach(func() {
		app := v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "vela-system", Name: "addon-test-view"}}
		Expect(k8sClient.Delete(ctx, &app)).Should(SatisfyAny(BeNil(), util.NotFoundMatcher{}))
	})

	It("test enable addon which applies the views independently", func() {
		ctx := context.Background()
		err := EnableAddonByLocalDir(ctx, "test-view", "./testdata/test-view", k8sClient, dc, apply.NewAPIApplicator(k8sClient), cfg, map[string]interface{}{"example": "test"})
		Expect(err).Should(BeNil())
		app := v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types2.NamespacedName{Namespace: "vela-system", Name: "addon-test-view"}, &app)).Should(BeNil())
		configMap := v1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types2.NamespacedName{Namespace: "vela-system", Name: "pod-view"}, &configMap)).Should(BeNil())
	})
})

const (
	appYaml = `apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: addon-test-app
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
`
	legacyAppYaml = `apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: legacy-addon
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
`
	deployYaml = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubevela-vela-core
  namespace: vela-system
  labels:
     controller.oam.dev/name: vela-core
spec:
  progressDeadlineSeconds: 600
  replicas: 1
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app.kubernetes.io/instance: kubevela
      app.kubernetes.io/name: vela-core
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      annotations:
        prometheus.io/path: /metrics
        prometheus.io/port: "8080"
        prometheus.io/scrape: "true"
      labels:
        app.kubernetes.io/instance: kubevela
        app.kubernetes.io/name: vela-core
    spec:
      containers:
      - args:
        image: oamdev/vela-core:v1.2.3
        imagePullPolicy: Always
        name: kubevela
        ports:
        - containerPort: 9443
          name: webhook-server
          protocol: TCP
        - containerPort: 9440
          name: healthz
          protocol: TCP
        resources:
          limits:
            cpu: 500m
            memory: 1Gi
          requests:
            cpu: 50m
            memory: 20Mi
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 30`

	addonUpdateAppYaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: addon-test-update
  namespace: vela-system
  labels:
    addons.oam.dev/version: v1.2.0
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8000
`
)
