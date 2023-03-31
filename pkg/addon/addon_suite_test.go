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
	"io"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	yaml3 "gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	"github.com/oam-dev/kubevela/references/cli/top/model"
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
			checkApp.Status.Workflow = &common.WorkflowStatus{
				Suspend: true,
			}
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
			checkApp.Status.Workflow = &common.WorkflowStatus{
				Message:     "someMessage",
				AppRevision: "test-revision",
			}
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

	It("checkDependencyNeedInstall func test", func() {
		// case1: dependency addon not enable
		depAddonName := "legacy-addon"
		addonClusters := []string{"cluster1", "cluster2"}
		needInstallAddonDep, depClusters, err := checkDependencyNeedInstall(ctx, k8sClient, depAddonName, addonClusters)
		Expect(needInstallAddonDep).Should(BeTrue())
		Expect(depClusters).Should(Equal(addonClusters))
		Expect(err).Should(BeNil())

		// case2: dependency addon enable
		app = v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(legacyAppYaml), &app)).Should(BeNil())
		app.SetNamespace(testns)
		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())
		Eventually(func(g Gomega) {
			needInstallAddonDep, depClusters, err := checkDependencyNeedInstall(ctx, k8sClient, depAddonName, addonClusters)
			Expect(err).Should(BeNil())
			Expect(needInstallAddonDep).Should(BeTrue())
			Expect(depClusters).Should(Equal(addonClusters))
		}, 30*time.Second).Should(Succeed())
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
		_, _, err := RenderApp(ctx, i, k8sClient, args)
		Expect(err.Error()).Should(BeEquivalentTo("cluster ne not exist"))
	})
	It("test render normal addon with specified clusters", func() {
		i := &baseAddon
		i.DeployTo = &DeployTo{RuntimeCluster: true}
		i.Name = "test-cluster-addon-normal"
		args := map[string]interface{}{
			"clusters": []string{"add-c1", "add-c2"},
		}
		ap, _, err := RenderApp(ctx, i, k8sClient, args)
		Expect(err).Should(BeNil())
		Expect(ap.Spec.Policies).Should(BeEquivalentTo([]v1beta1.AppPolicy{{Name: specifyAddonClustersTopologyPolicy,
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
		h := NewAddonInstaller(context.Background(), k8sClient, nil, nil, nil, &Registry{Name: "test"}, nil, nil, nil)
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
		_, err := EnableAddonByLocalDir(ctx, "example", "./testdata/example", k8sClient, dc, apply.NewAPIApplicator(k8sClient), cfg, map[string]interface{}{"example": "test"})
		Expect(err).Should(BeNil())
		app := v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types2.NamespacedName{Namespace: "vela-system", Name: "addon-example"}, &app)).Should(BeNil())
	})
})

var _ = Describe("test dry-run addon from local dir", func() {
	BeforeEach(func() {
		app := v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "vela-system", Name: "addon-example"}}
		Expect(k8sClient.Delete(ctx, &app)).Should(SatisfyAny(BeNil(), util.NotFoundMatcher{}))
		cd := v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Namespace: "vela-system", Name: "helm-example"}}
		Expect(k8sClient.Delete(ctx, &cd)).Should(SatisfyAny(BeNil(), util.NotFoundMatcher{}))
	})
	AfterEach(func() {
		app := v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "vela-system", Name: "addon-example"}}
		Expect(k8sClient.Delete(ctx, &app)).Should(SatisfyAny(BeNil(), util.NotFoundMatcher{}))

		cd := v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Namespace: "vela-system", Name: "helm-example"}}
		Expect(k8sClient.Delete(ctx, &cd)).Should(SatisfyAny(BeNil(), util.NotFoundMatcher{}))
	})

	It("test dry-run enable addon from local dir", func() {
		ctx := context.Background()

		r := localReader{dir: "./testdata/example", name: "addon-example"}
		metas, err := r.ListAddonMeta()
		Expect(err).Should(BeNil())

		meta := metas[r.name]
		UIData, err := GetUIDataFromReader(r, &meta, UIMetaOptions)
		Expect(err).Should(BeNil())

		pkg, err := GetInstallPackageFromReader(r, &meta, UIData)
		Expect(err).Should(BeNil())

		h := NewAddonInstaller(ctx, k8sClient, dc, apply.NewAPIApplicator(k8sClient), cfg, &Registry{Name: LocalAddonRegistryName}, map[string]interface{}{"example": "test-dry-run"}, nil, nil, DryRunAddon)

		_, err = h.enableAddon(pkg)
		Expect(err).Should(BeNil())

		decoder := yaml3.NewDecoder(h.dryRunBuff)
		for {
			obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
			err := decoder.Decode(obj.Object)
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				Expect(err).Should(BeNil())
			}
			Expect(obj.GetNamespace()).Should(BeEquivalentTo(model.VelaSystemNS))
			Expect(k8sClient.Create(ctx, obj)).Should(BeNil())
		}

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
		_, err := EnableAddonByLocalDir(ctx, "test-view", "./testdata/test-view", k8sClient, dc, apply.NewAPIApplicator(k8sClient), cfg, map[string]interface{}{"example": "test"})
		Expect(err).Should(BeNil())
		app := v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types2.NamespacedName{Namespace: "vela-system", Name: "addon-test-view"}, &app)).Should(BeNil())
		configMap := v1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types2.NamespacedName{Namespace: "vela-system", Name: "pod-view"}, &configMap)).Should(BeNil())
	})
})

var _ = Describe("test enable addon with notes", func() {
	BeforeEach(func() {
		app := v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Namespace: "vela-system", Name: "addon-test-notes"}}
		Expect(k8sClient.Delete(ctx, &app)).Should(SatisfyAny(BeNil(), util.NotFoundMatcher{}))
		sec := v1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "vela-system", Name: "addon-secret-test-notes"}}
		Expect(k8sClient.Delete(ctx, &sec)).Should(SatisfyAny(BeNil(), util.NotFoundMatcher{}))
	})

	It("test 'enable' addon which render notes output", func() {
		ctx := context.Background()
		addonInputArgs := map[string]interface{}{"example": "test"}
		// inject runtime info
		addonInputArgs[InstallerRuntimeOption] = map[string]interface{}{
			"upgrade": false,
		}
		notes, err := EnableAddonByLocalDir(ctx, "test-notes", "./testdata/test-notes", k8sClient, dc, apply.NewAPIApplicator(k8sClient), cfg, addonInputArgs)
		Expect(err).Should(BeNil())
		app := v1beta1.Application{}
		Expect(k8sClient.Get(ctx, types2.NamespacedName{Namespace: "vela-system", Name: "addon-test-notes"}, &app)).Should(BeNil())
		Expect(notes).Should(ContainSubstring(`Thank you for your first installation!
Please refer to URL.`))
	})

	It("test 'upgrade' addon which render notes output", func() {
		ctx := context.Background()
		addonInputArgs := map[string]interface{}{"example": "test"}
		// inject runtime info
		addonInputArgs[InstallerRuntimeOption] = map[string]interface{}{
			"upgrade": true,
		}
		notes, err := EnableAddonByLocalDir(ctx, "test-notes-upgrade", "./testdata/test-notes", k8sClient, dc, apply.NewAPIApplicator(k8sClient), cfg, addonInputArgs)
		Expect(err).Should(BeNil())
		Expect(notes).Should(ContainSubstring(`Thank you for your upgrade!
Please refer to URL.`))
	})
})

var _ = Describe("test override defs of addon", func() {
	It("test compDef exist", func() {
		ctx := context.Background()
		comp := v1beta1.ComponentDefinition{TypeMeta: metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.ComponentDefinitionKind}}
		Expect(yaml.Unmarshal([]byte(helmCompDefYaml), &comp)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &comp)).Should(BeNil())

		comp2 := v1beta1.ComponentDefinition{TypeMeta: metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.ComponentDefinitionKind}}
		Expect(yaml.Unmarshal([]byte(kustomizeCompDefYaml), &comp2)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &comp2)).Should(BeNil())
		app := v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "addon-fluxcd"}}

		comp3 := v1beta1.ComponentDefinition{TypeMeta: metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.ComponentDefinitionKind}}
		Expect(yaml.Unmarshal([]byte(kustomizeCompDefYaml1), &comp3)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &comp3)).Should(BeNil())

		compUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&comp)
		Expect(err).Should(BeNil())
		u := unstructured.Unstructured{Object: compUnstructured}
		u.SetAPIVersion(v1beta1.SchemeGroupVersion.String())
		u.SetKind(v1beta1.ComponentDefinitionKind)
		u.SetLabels(map[string]string{"testUpdateLabel": "test"})
		c, err := checkConflictDefs(ctx, k8sClient, []*unstructured.Unstructured{&u}, app.GetName())
		Expect(err).Should(BeNil())
		Expect(len(c)).Should(BeEquivalentTo(1))
		// guarantee checkConflictDefs won't change source definition
		Expect(u.GetLabels()["testUpdateLabel"]).Should(BeEquivalentTo("test"))

		u.SetName("rollout")
		c, err = checkConflictDefs(ctx, k8sClient, []*unstructured.Unstructured{&u}, app.GetName())
		Expect(err).Should(BeNil())
		Expect(len(c)).Should(BeEquivalentTo(0))

		u.SetKind("NotExistKind")
		_, err = checkConflictDefs(ctx, k8sClient, []*unstructured.Unstructured{&u}, app.GetName())
		Expect(err).ShouldNot(BeNil())

		compUnstructured2, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&comp2)
		Expect(err).Should(BeNil())
		u2 := &unstructured.Unstructured{Object: compUnstructured2}
		u2.SetAPIVersion(v1beta1.SchemeGroupVersion.String())
		u2.SetKind(v1beta1.ComponentDefinitionKind)
		c, err = checkConflictDefs(ctx, k8sClient, []*unstructured.Unstructured{u2}, app.GetName())
		Expect(err).Should(BeNil())
		Expect(len(c)).Should(BeEquivalentTo(1))

		compUnstructured3, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&comp3)
		Expect(err).Should(BeNil())
		u3 := &unstructured.Unstructured{Object: compUnstructured3}
		u3.SetAPIVersion(v1beta1.SchemeGroupVersion.String())
		u3.SetKind(v1beta1.ComponentDefinitionKind)
		c, err = checkConflictDefs(ctx, k8sClient, []*unstructured.Unstructured{u3}, app.GetName())
		Expect(err).Should(BeNil())
		Expect(len(c)).Should(BeEquivalentTo(0))
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
	helmCompDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: helm
  namespace: vela-system
  ownerReferences:
  - apiVersion: core.oam.dev/v1beta1
    blockOwnerDeletion: true
    controller: true
    kind: Application
    name: addon-fluxcd-helm
    uid: 73c47933-002e-4182-a673-6da6a9dcf080
spec:
  schematic:
    cue:
`
	kustomizeCompDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: kustomize
  namespace: vela-system
spec:
  schematic:
    cue:
`
	kustomizeCompDefYaml1 = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: kustomize-another
  namespace: vela-system
  ownerReferences:
  - apiVersion: core.oam.dev/v1beta1
    blockOwnerDeletion: true
    controller: true
    kind: Application
    name: addon-fluxcd
    uid: 73c47933-002e-4182-a673-6da6a9dcf080
spec:
  schematic:
    cue:
`
)
