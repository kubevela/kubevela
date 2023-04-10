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

package application

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	sysruntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/oam-dev/kubevela/pkg/features"

	"github.com/google/go-cmp/cmp"
	testdef "github.com/kubevela/pkg/util/test/definition"
	wffeatures "github.com/kubevela/workflow/pkg/features"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/kubevela/workflow/pkg/debug"
	wfTypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	stdv1alpha1 "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/testutil"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

// TODO: Refactor the tests to not copy and paste duplicated code 10 times
var _ = Describe("Test Application Controller", func() {
	ctx := context.TODO()
	appwithNoTrait := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-with-no-trait",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "myweb2",
					Type:       "worker",
					Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
				},
			},
		},
	}

	appFailParse := appwithNoTrait.DeepCopy()
	appFailParse.SetName("app-fail-to-parsed")
	appFailParse.Spec.Components[0].Type = "fakeWorker"

	appFailRender := appwithNoTrait.DeepCopy()
	appFailRender.SetName("app-fail-to-render")
	appFailRender.Spec.Components[0].Properties = &runtime.RawExtension{
		Raw: []byte(`{"cmd1":["sleep","1000"],"image1":"busybox"}`),
	}
	appFailRender.Spec.Policies = []v1beta1.AppPolicy{
		{
			Name:       "policy1",
			Type:       "foopolicy",
			Properties: &runtime.RawExtension{Raw: []byte(`{"test":"test"}`)},
		},
	}

	var getExpDeployment = func(compName string, app *v1beta1.Application) *v1.Deployment {
		var namespace = app.Namespace
		if namespace == "" {
			namespace = "default"
		}
		return &v1.Deployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"workload.oam.dev/type":    "worker",
					"app.oam.dev/component":    compName,
					"app.oam.dev/name":         app.Name,
					"app.oam.dev/namespace":    app.Namespace,
					"app.oam.dev/appRevision":  app.Name + "-v1",
					"app.oam.dev/resourceType": "WORKLOAD",
				},
				Name:      compName,
				Namespace: namespace,
			},
			Spec: v1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
					"app.oam.dev/component": compName,
				}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
						"app.oam.dev/component": compName,
					}},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Image:   "busybox",
						Name:    compName,
						Command: []string{"sleep", "1000"},
					},
					}}},
			},
		}
	}

	appWithTrait := appwithNoTrait.DeepCopy()
	appWithTrait.SetName("app-with-trait")
	appWithTrait.Spec.Components[0].Traits = []common.ApplicationTrait{
		{
			Type:       "scaler",
			Properties: &runtime.RawExtension{Raw: []byte(`{"replicas":2}`)},
		},
	}
	appWithTrait.Spec.Components[0].Name = "myweb3"

	appWithTraitAndScope := appWithTrait.DeepCopy()
	appWithTraitAndScope.SetName("app-with-trait-and-scope")
	appWithTraitAndScope.Spec.Components[0].Scopes = map[string]string{"healthscopes.core.oam.dev": "appWithTraitAndScope-default-health"}
	appWithTraitAndScope.Spec.Components[0].Name = "myweb4"

	appWithTwoComp := appWithTraitAndScope.DeepCopy()
	appWithTwoComp.SetName("app-with-two-comp")
	appWithTwoComp.Spec.Components[0].Scopes = map[string]string{"healthscopes.core.oam.dev": "app-with-two-comp-default-health"}
	appWithTwoComp.Spec.Components[0].Name = "myweb5"
	appWithTwoComp.Spec.Components = append(appWithTwoComp.Spec.Components, common.ApplicationComponent{
		Name:       "myweb6",
		Type:       "worker",
		Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox2","config":"myconfig"}`)},
		Scopes:     map[string]string{"healthscopes.core.oam.dev": "app-with-two-comp-default-health"},
	})

	appWithStorage := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-storage",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "myworker",
					Type:       "worker",
					Properties: &runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\",\"env\":[{\"name\":\"firstKey\",\"value\":\"firstValue\"}]}")},
				},
			},
		},
	}
	appWithStorage.Spec.Components[0].Traits = []common.ApplicationTrait{
		{
			Type:       "storage",
			Properties: &runtime.RawExtension{Raw: []byte("{\"configMap\":[{\"name\":\"myworker-cm\",\"mountPath\":\"/test/mount/cm\",\"data\":{\"secondKey\":\"secondValue\"}}]}")},
		},
		{
			Type:       "env",
			Properties: &runtime.RawExtension{Raw: []byte("{\"env\":{\"firstKey\":\"newValue\"}}")},
		},
	}

	appWithHttpsGateway := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-gateway",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "myworker",
					Type:       "worker",
					Properties: &runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\"}")},
				},
			},
		},
	}
	appWithHttpsGateway.Spec.Components[0].Traits = []common.ApplicationTrait{
		{
			Type:       "gateway",
			Properties: &runtime.RawExtension{Raw: []byte(`{"secretName":"myworker-secret","domain":"example.com","http":{"/":80}}`)},
		},
	}

	appWithMountPath := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-storage",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "myworker",
					Type:       "worker",
					Properties: &runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\"}")},
				},
			},
		},
	}
	appWithMountPath.Spec.Components[0].Traits = []common.ApplicationTrait{
		{
			Type:       "storage",
			Properties: &runtime.RawExtension{Raw: []byte("{\"secret\":[{\"name\":\"myworker-secret\",\"mountToEnv\":{\"envName\":\"firstEnv\",\"secretKey\":\"firstKey\"},\"data\":{\"firstKey\":\"dmFsdWUwMQo=\"}}]}")},
		},
		{
			Type:       "storage",
			Properties: &runtime.RawExtension{Raw: []byte("{\"configMap\":[{\"name\": \"myworker-cm\",\"mountToEnv\":{ \"envName\":\"secondEnv\",\"configMapKey\":\"secondKey\"},\"data\": {\"secondKey\":\"secondValue\"}}]}")},
		},
	}

	appWithControlPlaneOnly := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-controlplaneonly",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "app-controlplaneonly-component",
					Type:       "worker",
					Properties: &runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\"}")},
					Traits: []common.ApplicationTrait{
						{
							Type:       "hubcpuscaler",
							Properties: &runtime.RawExtension{Raw: []byte("{\"min\": 1,\"max\": 10,\"cpuPercent\": 60}")},
						},
					},
				},
			},
		},
	}

	appWithHttpsHealthProbe := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-httphealthprobe",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "app-httphealthprobe-component",
					Type:       "worker",
					Properties: &runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\",\"livenessProbe\":{\"failureThreshold\":3,\"httpGet\":{\"path\":\"/v1/health\",\"port\":8080,\"scheme\":\"HTTP\"},\"initialDelaySeconds\":60,\"periodSeconds\":60,\"successThreshold\":1,\"timeoutSeconds\":5}}")},
				},
			},
		},
	}

	appWithApplyOnce := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-apply-once",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "app-applyonce-component",
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
		},
	}
	appWithApplyOnce.Spec.Components[0].Traits = []common.ApplicationTrait{
		{
			Type:       "scaler",
			Properties: &runtime.RawExtension{Raw: []byte(`{"replicas":2}`)},
		},
	}

	appWithMountToEnvs := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-with-mount-to-envs",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "myweb",
					Type:       "worker",
					Properties: &runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\"}")},
				},
			},
		},
	}
	appWithMountToEnvs.Spec.Components[0].Traits = []common.ApplicationTrait{
		{
			Type:       "storage",
			Properties: &runtime.RawExtension{Raw: []byte("{\"secret\": [{\"name\": \"myweb-secret\",\"mountToEnv\": {\"envName\": \"firstEnv\",\"secretKey\": \"firstKey\"},\"mountToEnvs\": [{\"envName\": \"secondEnv\",\"secretKey\": \"secondKey\"}],\"data\": {\"firstKey\": \"dmFsdWUwMQo=\",\"secondKey\": \"dmFsdWUwMgo=\"}}]}")},
		},
		{
			Type:       "storage",
			Properties: &runtime.RawExtension{Raw: []byte("{\"configMap\": [{\"name\": \"myweb-cm\",\"mountToEnvs\": [{\"envName\":\"thirdEnv\",\"configMapKey\":\"thirdKey\"},{\"envName\":\"fourthEnv\",\"configMapKey\":\"fourthKey\"}],\"data\": {\"thirdKey\": \"Value03\",\"fourthKey\": \"Value04\"}}]}")},
		},
	}

	appWithAffinity := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-with-affinity",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "myweb",
					Type:       "worker",
					Properties: &runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\"}")},
				},
			},
		},
	}
	appWithAffinity.Spec.Components[0].Traits = []common.ApplicationTrait{
		{
			Type:       "affinity",
			Properties: &runtime.RawExtension{Raw: []byte("{\"podAffinity\":{\"preferred\":[{\"podAffinityTerm\":{\"labelSelector\":{\"matchExpressions\":[{\"key\": \"security\",\"values\": [\"S1\"]}]},\"namespaces\":[\"default\"],\"topologyKey\": \"kubernetes.io/hostname\"},\"weight\": 1}]}}")},
		},
	}

	cd := &v1beta1.ComponentDefinition{}
	cDDefJson, _ := yaml.YAMLToJSON([]byte(componentDefYaml))
	k8sObjectsCDJson, _ := yaml.YAMLToJSON([]byte(k8sObjectsComponentDefinitionYaml))

	pd := &v1beta1.PolicyDefinition{}
	pd.Namespace = "vela-system"
	pdDefJson, _ := yaml.YAMLToJSON([]byte(policyDefYaml))

	importWd := &v1beta1.WorkloadDefinition{}
	importWdJson, _ := yaml.YAMLToJSON([]byte(wDImportYaml))

	importTd := &v1alpha2.TraitDefinition{}

	webserverwd := &v1alpha2.ComponentDefinition{}
	webserverwdJson, _ := yaml.YAMLToJSON([]byte(webComponentDefYaml))

	sd := &v1beta1.ScopeDefinition{}
	sdDefJson, _ := yaml.YAMLToJSON([]byte(scopeDefYaml))

	BeforeEach(func() {
		Expect(json.Unmarshal(cDDefJson, cd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, cd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(k8sObjectsCDJson, cd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, cd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(pdDefJson, pd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, pd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(importWdJson, importWd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, importWd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		importTdJson, err := yaml.YAMLToJSON([]byte(tdImportedYaml))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(json.Unmarshal(importTdJson, importTd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, importTd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		_, file, _, _ := sysruntime.Caller(0)
		for _, trait := range []string{"gateway", "storage", "env", "affinity", "scaler"} {
			Expect(testdef.InstallDefinitionFromYAML(ctx, k8sClient, filepath.Join(file, "../../../../../../charts/vela-core/templates/defwithtemplate/", trait+".yaml"), func(s string) string {
				return strings.ReplaceAll(s, `{{ include "systemDefinitionNamespace" . }}`, "vela-system")
			})).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		}
		for _, def := range []string{"panic", "hubcpuscaler", "storage-pre-dispatch", "storage-pre-dispatch-unhealthy"} {
			Expect(testdef.InstallDefinitionFromYAML(ctx, k8sClient, filepath.Join(file, "../../application/testdata/definitions/", def+".yaml"), nil)).
				Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		}

		Expect(json.Unmarshal(sdDefJson, sd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, sd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(webserverwdJson, webserverwd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, webserverwd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	})

	It("app step will set event", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-app-without-trait-event",
			},
		}
		appwithNoTrait.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, appwithNoTrait.DeepCopy())).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      appwithNoTrait.Name,
			Namespace: appwithNoTrait.Namespace,
		}

		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		events, err := recorder.GetEventsWithName(appwithNoTrait.Name)
		Expect(err).Should(BeNil())
		Expect(len(events)).ShouldNot(Equal(0))
		for _, event := range events {
			Expect(event.EventType).ShouldNot(Equal(corev1.EventTypeWarning))
			Expect(event.EventType).Should(Equal(corev1.EventTypeNormal))
		}

		// fail to parse application
		appFailParse.SetNamespace(ns.Name)
		appFailParseKey := client.ObjectKey{
			Name:      appFailParse.Name,
			Namespace: appFailParse.Namespace,
		}

		Expect(k8sClient.Create(ctx, appFailParse.DeepCopy())).Should(BeNil())
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appFailParseKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appFailParseKey})

		parseEvents, err := recorder.GetEventsWithName(appFailParse.Name)
		Expect(err).Should(BeNil())
		Expect(len(parseEvents)).Should(Equal(1))
		for _, event := range parseEvents {
			Expect(event.EventType).Should(Equal(corev1.EventTypeWarning))
			Expect(event.Reason).Should(Equal(velatypes.ReasonFailedParse))
		}

		// fail to render application
		appFailRender.SetNamespace(ns.Name)
		appFailRenderKey := client.ObjectKey{
			Name:      appFailRender.Name,
			Namespace: appFailRender.Namespace,
		}
		Expect(k8sClient.Create(ctx, appFailRender.DeepCopy())).Should(BeNil())
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appFailRenderKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appFailRenderKey})

		renderEvents, err := recorder.GetEventsWithName(appFailRender.Name)
		Expect(err).Should(BeNil())
		Expect(len(renderEvents)).Should(Equal(3))
	})

	It("app-without-trait will only create workload", func() {

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-app-without-trait",
			},
		}
		appwithNoTrait.SetNamespace(ns.Name)
		expDeployment := getExpDeployment("myweb2", appwithNoTrait)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, appwithNoTrait.DeepCopy())).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      appwithNoTrait.Name,
			Namespace: appwithNoTrait.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		By("Check Application Created")
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		By("Check affiliated resource tracker is created")
		expectRTName := fmt.Sprintf("%s-%s", checkApp.Status.LatestRevision.Name, checkApp.GetNamespace())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: expectRTName}, &v1beta1.ResourceTracker{})
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("check AppRevision created with the expected workload spec")
		appRev := &v1beta1.ApplicationRevision{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: checkApp.Name + "-v1", Namespace: checkApp.GetNamespace()}, appRev)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		af, err := appParser.GenerateAppFileFromRevision(appRev)
		Expect(err).Should(BeNil())
		comps, err := af.GenerateComponentManifests()
		Expect(err).Should(BeNil())
		Expect(len(comps) > 0).Should(BeTrue())
		comp := comps[0]

		Expect(comp.StandardWorkload).ShouldNot(BeNil())
		gotDeploy := &v1.Deployment{}
		Expect(runtime.DefaultUnstructuredConverter.FromUnstructured(comp.StandardWorkload.Object, gotDeploy)).Should(Succeed())
		gotDeploy.Annotations = nil
		Expect(cmp.Diff(gotDeploy, expDeployment)).Should(BeEmpty())

		By("Delete Application, clean the resource")
		Expect(k8sClient.Delete(ctx, appwithNoTrait)).Should(BeNil())
	})

	It("app with health policy and custom status for workload", func() {
		By("change workload and trait definition with health policy")
		ncd := &v1beta1.ComponentDefinition{}
		cDDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(cDDefJson, ncd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ncd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		ntd := &v1beta1.TraitDefinition{}
		tDDefJson, _ := yaml.YAMLToJSON([]byte(tDDefWithHealthStatusYaml))
		Expect(json.Unmarshal(tDDefJson, ntd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ntd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		compName := "myweb-health-status"
		appWithTraitHealthStatus := appWithTrait.DeepCopy()
		appWithTraitHealthStatus.Name = "app-trait-health-status"

		By("create the new namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-health-status",
			},
		}
		appWithTraitHealthStatus.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		app := appWithTraitHealthStatus.DeepCopy()
		app.Spec.Components[0].Name = compName
		app.Spec.Components[0].Type = "nworker"
		app.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox3","lives":"3","enemies":"alien"}`)}
		app.Spec.Components[0].Traits[0].Type = "ingress"
		app.Spec.Components[0].Traits[0].Properties = &runtime.RawExtension{Raw: []byte(`{"domain":"example.com","http":{"/":80}}`)}

		By("apply appfile")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		deploy := &v1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns.Name, Name: "myweb-health-status"}, deploy)).Should(Succeed())
		deploy.Status.Replicas = 1
		deploy.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, deploy)).Should(Succeed())
		svcs := &corev1.ServiceList{}
		Expect(k8sClient.List(ctx, svcs, client.InNamespace(ns.Name))).Should(Succeed())
		Expect(len(svcs.Items)).Should(Equal(1))
		clusterIP := svcs.Items[0].Spec.ClusterIP

		By("Check App running successfully")
		checkApp := &v1beta1.Application{}
		Eventually(func() string {
			_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: appKey})
			if err != nil {
				return err.Error()
			}
			err = k8sClient.Get(ctx, appKey, checkApp)
			if err != nil {
				return err.Error()
			}
			if checkApp.Status.Phase != common.ApplicationRunning {
				fmt.Println(checkApp.Status.Conditions)
			}
			return string(checkApp.Status.Phase)
		}, 5*time.Second, time.Second).Should(BeEquivalentTo(common.ApplicationRunning))
		Expect(checkApp.Status.Services).Should(BeEquivalentTo([]common.ApplicationComponentStatus{
			{
				Name:               compName,
				Namespace:          app.Namespace,
				WorkloadDefinition: ncd.Spec.Workload.Definition,
				Healthy:            true,
				Message:            "type: busybox3,\t enemies:alien",
				Traits: []common.ApplicationTraitStatus{
					{
						Type:    "ingress",
						Healthy: true,
						Message: fmt.Sprintf("type: ClusterIP,\t clusterIP:%s,\t ports:80,\t domainexample.com", clusterIP),
					},
				},
			},
		}))
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app with a component refer to an existing WorkloadDefinition", func() {
		appRefertoWd := appwithNoTrait.DeepCopy()
		appRefertoWd.Spec.Components[0] = common.ApplicationComponent{
			Name:       "mytask",
			Type:       "task",
			Properties: &runtime.RawExtension{Raw: []byte(`{"image":"busybox", "cmd":["sleep","1000"]}`)},
		}
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-app-with-workload-task",
			},
		}
		appRefertoWd.SetName("test-app-with-workload-task")
		appRefertoWd.SetNamespace(ns.Name)

		taskWd := &v1beta1.WorkloadDefinition{}
		wDDefJson, _ := yaml.YAMLToJSON([]byte(workloadDefYaml))
		Expect(json.Unmarshal(wDDefJson, taskWd)).Should(BeNil())
		taskWd.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, taskWd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, appRefertoWd.DeepCopy())).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      appRefertoWd.Name,
			Namespace: appRefertoWd.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		By("Check Application Created with the correct revision")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))
		Expect(curApp.Status.LatestRevision).ShouldNot(BeNil())
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))

		By("Check AppRevision created as expected")
		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: curApp.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())

		By("Check affiliated resource tracker is created")
		expectRTName := fmt.Sprintf("%s-%s", appRevision.GetName(), appRevision.GetNamespace())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: expectRTName}, &v1beta1.ResourceTracker{})
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())
	})

	It("app with two components and one component refer to an existing WorkloadDefinition", func() {
		appMix := appWithTwoComp.DeepCopy()
		appMix.Spec.Components[1] = common.ApplicationComponent{
			Name:       "mytask",
			Type:       "task",
			Properties: &runtime.RawExtension{Raw: []byte(`{"image":"busybox", "cmd":["sleep","1000"]}`)},
		}
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-app-with-mix-components",
			},
		}
		appMix.SetName("test-app-with-mix-components")
		appMix.SetNamespace(ns.Name)

		taskWd := &v1beta1.WorkloadDefinition{}
		wDDefJson, _ := yaml.YAMLToJSON([]byte(workloadDefYaml))
		Expect(json.Unmarshal(wDDefJson, taskWd)).Should(BeNil())
		taskWd.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, taskWd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, appMix.DeepCopy())).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      appMix.Name,
			Namespace: appMix.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		By("Check Application Created with the correct revision")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))
		Expect(curApp.Status.LatestRevision).ShouldNot(BeNil())
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))

		By("Check AppRevision created as expected")
		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: curApp.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())
	})

	It("revision should be updated if the workflow is restarted", func() {

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-app-restart-revision",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vela-test-app-restart-revision",
				Namespace: "vela-test-app-restart-revision",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "suspend",
								Type: "suspend",
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, app.DeepCopy())).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		By("Check Application Created with the correct revision")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationWorkflowSuspending))
		Expect(curApp.Status.LatestRevision).ShouldNot(BeNil())
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))

		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())
		Expect(appRevision.Status.Workflow).Should(BeNil())

		// update the app
		curApp.Spec.Workflow.Steps[0].DependsOn = []string{"invalid"}
		Expect(k8sClient.Update(ctx, curApp)).Should(BeNil())
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())
		Expect(appRevision.Status.Workflow).ShouldNot(BeNil())
		Expect(appRevision.Status.Workflow.Finished).Should(BeTrue())
		Expect(appRevision.Status.Workflow.Terminated).Should(BeTrue())
		Expect(appRevision.Status.Workflow.EndTime.IsZero()).ShouldNot(BeTrue())
		Expect(appRevision.Status.Workflow.Phase).Should(Equal(workflowv1alpha1.WorkflowStateSuspending))

		By("Delete Application, clean the resource")
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("revision should exist in created workload render by context.appRevision", func() {

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-app-revisionname",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		cd := &v1beta1.ComponentDefinition{}
		Expect(common2.ReadYamlToObject("testdata/revision/cd1.yaml", cd)).Should(BeNil())
		cd.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, cd.DeepCopy())).Should(BeNil())

		app := &v1beta1.Application{}
		Expect(common2.ReadYamlToObject("testdata/revision/app1.yaml", app)).Should(BeNil())
		app.SetNamespace(ns.Name)

		expDeployment := getExpDeployment("myweb", app)
		expDeployment.Labels["workload.oam.dev/type"] = "cd1"
		expDeployment.Spec.Template.Spec.Containers[0].Command = nil
		expDeployment.Spec.Template.Labels["app.oam.dev/revision"] = "revision-app1-v1"

		Expect(k8sClient.Create(ctx, app.DeepCopy())).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		By("Check Application Created with the correct revision")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))
		Expect(curApp.Status.LatestRevision).ShouldNot(BeNil())
		Expect(curApp.Status.LatestRevision.Revision).Should(BeEquivalentTo(1))

		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())

		af, err := appParser.GenerateAppFileFromRevision(appRevision)
		Expect(err).Should(BeNil())
		comps, err := af.GenerateComponentManifests()
		Expect(err).Should(BeNil())
		Expect(len(comps) > 0).Should(BeTrue())
		comp := comps[0]

		gotD := &v1.Deployment{}
		runtime.DefaultUnstructuredConverter.FromUnstructured(comp.StandardWorkload.Object, gotD)
		gotD.Annotations = nil
		Expect(cmp.Diff(gotD, expDeployment)).Should(BeEmpty())

		By("Delete Application, clean the resource")
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("Test rollout trait all related definition features", func() {
		rolloutTdDef, err := yaml.YAMLToJSON([]byte(rolloutTraitDefinition))
		Expect(err).Should(BeNil())
		rolloutTrait := &v1beta1.TraitDefinition{}
		Expect(json.Unmarshal([]byte(rolloutTdDef), rolloutTrait)).Should(BeNil())
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-rollout-trait",
			},
		}
		rolloutTrait.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, rolloutTrait)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-rollout",
				Namespace: ns.Name,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						Traits: []common.ApplicationTrait{
							{
								Type: "rollout",
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
		checkRollout := &stdv1alpha1.Rollout{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "myweb1", Namespace: ns.Name}, checkRollout)).Should(BeNil())
		By("verify targetRevision will be filled with real compRev by context.ComponentRevName")
		Expect(checkRollout.Spec.TargetRevisionName).Should(BeEquivalentTo("myweb1-v1"))
		deploy := &v1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "myweb1", Namespace: ns.Name}, deploy)).Should(util.NotFoundMatcher{})

		By("update component targetComponentRev will change")
		checkApp.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","2000"],"image":"nginx"}`)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp = &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
		checkRollout = &stdv1alpha1.Rollout{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "myweb1", Namespace: ns.Name}, checkRollout)).Should(BeNil())
		By("verify targetRevision will be filled with newest")
		Expect(checkRollout.Spec.TargetRevisionName).Should(BeEquivalentTo("myweb1-v2"))
		deploy = &v1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "myweb1", Namespace: ns.Name}, deploy)).Should(util.NotFoundMatcher{})

		By("check update rollout trait won't generate new appRevision")
		checkApp.Spec.Components[0].Traits[0].Properties = &runtime.RawExtension{Raw: []byte(`{"targetRevision":"myweb1-v3"}`)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp = &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.LatestRevision.Name).Should(BeEquivalentTo("app-with-rollout-v3"))
		checkRollout = &stdv1alpha1.Rollout{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "myweb1", Namespace: ns.Name}, checkRollout)).Should(BeNil())
		Expect(checkRollout.Spec.TargetRevisionName).Should(BeEquivalentTo("myweb1-v3"))
	})

	It("Test context revision can be supported by specify externalRevision ", func() {
		rolloutTdDef, err := yaml.YAMLToJSON([]byte(rolloutTraitDefinition))
		Expect(err).Should(BeNil())
		rolloutTrait := &v1beta1.TraitDefinition{}
		externalRevision := "my-test-revision-v1"
		Expect(json.Unmarshal([]byte(rolloutTdDef), rolloutTrait)).Should(BeNil())
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-specify-external-revision",
			},
		}
		rolloutTrait.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, rolloutTrait)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-rollout",
				Namespace: ns.Name,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:             "myweb1",
						Type:             "worker",
						ExternalRevision: externalRevision,
						Properties:       &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						Traits: []common.ApplicationTrait{
							{
								Type: "rollout",
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
		checkRollout := &stdv1alpha1.Rollout{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "myweb1", Namespace: ns.Name}, checkRollout)).Should(BeNil())
		By("verify targetRevision will be filled with real compRev by context.Revision")
		Expect(checkRollout.Spec.TargetRevisionName).Should(BeEquivalentTo(externalRevision))
	})

	It("Test context revision can be supported by in  workload ", func() {
		compDef, err := yaml.YAMLToJSON([]byte(workloadWithContextRevision))
		Expect(err).Should(BeNil())
		component := &v1beta1.ComponentDefinition{}
		Expect(json.Unmarshal([]byte(compDef), component)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, component)).Should(BeNil())
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-workload-context-revision",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-test-context-revision",
				Namespace: ns.Name,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-revision",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
		deploy := &v1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "myweb1", Namespace: ns.Name}, deploy)).Should(BeNil())
		By("verify targetRevision will be filled with real compRev by context.Revision")
		Expect(len(deploy.Spec.Template.Labels)).Should(BeEquivalentTo(2))
		Expect(deploy.Spec.Template.Labels["app.oam.dev/revision"]).Should(BeEquivalentTo("myweb1-v1"))
	})

	It("Test context revision can be supported by in  workload when specified componentRevision", func() {
		compDef, err := yaml.YAMLToJSON([]byte(workloadWithContextRevision))
		Expect(err).Should(BeNil())
		component := &v1beta1.ComponentDefinition{}
		Expect(json.Unmarshal([]byte(compDef), component)).Should(BeNil())
		Expect(k8sClient.Create(ctx, component)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		externalRevision := "my-component-rev-v1"
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-workload-context-revision-specify-revision",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-test-context-revision",
				Namespace: ns.Name,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:             "myweb1",
						Type:             "worker-revision",
						ExternalRevision: externalRevision,
						Properties:       &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
		deploy := &v1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "myweb1", Namespace: ns.Name}, deploy)).Should(BeNil())
		By("verify targetRevision will be filled with real compRev by context.Revision")
		Expect(len(deploy.Spec.Template.Labels)).Should(BeEquivalentTo(2))
		Expect(deploy.Spec.Template.Labels["app.oam.dev/revision"]).Should(BeEquivalentTo(externalRevision))
	})

	It("Test rollout trait in workflow", func() {
		rolloutTdDef, err := yaml.YAMLToJSON([]byte(rolloutTraitDefinition))
		Expect(err).Should(BeNil())
		rolloutTrait := &v1beta1.TraitDefinition{}
		Expect(json.Unmarshal([]byte(rolloutTdDef), rolloutTrait)).Should(BeNil())

		wfStepDef, err := yaml.YAMLToJSON([]byte(applyCompWfStepDefinition))
		Expect(err).Should(BeNil())
		wfStep := &v1beta1.WorkflowStepDefinition{}
		Expect(json.Unmarshal([]byte(wfStepDef), wfStep)).Should(BeNil())

		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-rollout-workflow",
			},
		}
		rolloutTrait.SetNamespace(ns.Name)
		wfStep.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, rolloutTrait)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, wfStep)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-rollout-workflow",
				Namespace: ns.Name,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						Traits: []common.ApplicationTrait{
							{
								Type: "rollout",
							},
						},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "apply",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component" : "myweb1"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))

		By("verify workflow apply component had apply rollout")
		checkRollout := &stdv1alpha1.Rollout{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "myweb1", Namespace: ns.Name}, checkRollout)).Should(BeNil())
		By("verify targetRevision will be filled with real compRev by context.ComponentRevName")
		Expect(checkRollout.Spec.TargetRevisionName).Should(BeEquivalentTo("myweb1-v1"))

		By("verify workflow apply component didn't apply workload")
		deploy := &v1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "myweb1", Namespace: ns.Name}, deploy)).Should(util.NotFoundMatcher{})
	})

	It("application with dag workflow failed after retries", func() {
		defer featuregatetesting.SetFeatureGateDuringTest(&testing.T{}, utilfeature.DefaultFeatureGate, wffeatures.EnableSuspendOnFailure, true)()
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dag-failed-after-retries",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dag-failed-after-retries",
				Namespace: ns.Name,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       "failed-step",
						Type:       "k8s-objects",
						Properties: &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"invalid","metadata":{"name":"test1"}}]}`)},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp := &v1beta1.Application{}

		By("verify the first ten reconciles")
		for i := 0; i < wfTypes.MaxWorkflowStepErrorRetryTimes; i++ {
			testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
			Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
			Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
			Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(""))
			Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseFailed))
		}

		By("application should be suspended after failed max reconciles")
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowSuspending))
		Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(wfTypes.MessageSuspendFailedAfterRetries))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseFailed))
		Expect(checkApp.Status.Workflow.Steps[1].Reason).Should(BeEquivalentTo(wfTypes.StatusReasonFailedAfterRetries))

		By("resume the suspended application")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		checkApp.Status.Workflow.Suspend = false
		Expect(k8sClient.Status().Patch(ctx, checkApp, client.Merge)).Should(BeNil())
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
		Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(""))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseFailed))

		By("test failed-after-retries with running steps")
		compDef, err := yaml.YAMLToJSON([]byte(unhealthyComponentDefYaml))
		Expect(err).Should(BeNil())
		component := &v1beta1.ComponentDefinition{}
		component.Spec.Extension = &runtime.RawExtension{Raw: compDef}
		Expect(json.Unmarshal([]byte(compDef), component)).Should(BeNil())
		Expect(k8sClient.Create(ctx, component)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		checkApp.Spec.Components[0] = common.ApplicationComponent{
			Name:       "unhealthy-worker",
			Type:       "unhealthy-worker",
			Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
		}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))

		for i := 0; i < wfTypes.MaxWorkflowStepErrorRetryTimes-1; i++ {
			testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
			Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
			Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
			Expect(checkApp.Status.Workflow.Steps[0].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseRunning))
			Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseFailed))
		}

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
		Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(""))
		Expect(checkApp.Status.Workflow.Steps[0].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseRunning))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseFailed))
		Expect(checkApp.Status.Workflow.Steps[1].Reason).Should(BeEquivalentTo(wfTypes.StatusReasonFailedAfterRetries))
	})

	It("application with step by step workflow failed after retries", func() {
		defer featuregatetesting.SetFeatureGateDuringTest(&testing.T{}, utilfeature.DefaultFeatureGate, wffeatures.EnableSuspendOnFailure, true)()
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "step-by-step-failed-after-retries",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "step-by-step-failed-after-retries",
				Namespace: ns.Name,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       "failed-step",
						Type:       "k8s-objects",
						Properties: &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"invalid","metadata":{"name":"test1"}}]}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb1",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "failed-step",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"failed-step"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp := &v1beta1.Application{}

		By("verify the first twenty reconciles")
		for i := 0; i < wfTypes.MaxWorkflowStepErrorRetryTimes; i++ {
			testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
			Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
			Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
			Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(""))
			Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseFailed))
		}

		By("application should be suspended after failed max reconciles")
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowSuspending))
		Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(wfTypes.MessageSuspendFailedAfterRetries))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseFailed))
		Expect(checkApp.Status.Workflow.Steps[1].Reason).Should(BeEquivalentTo(wfTypes.StatusReasonFailedAfterRetries))

		By("resume the suspended application")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		checkApp.Status.Workflow.Suspend = false
		Expect(k8sClient.Status().Patch(ctx, checkApp, client.Merge)).Should(BeNil())
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
		Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(""))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseFailed))
	})

	It("application with mode in workflows", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-mode",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-mode"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-mode",
				Namespace: "app-with-mode",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb3",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Mode: &workflowv1alpha1.WorkflowExecuteMode{
						Steps:    workflowv1alpha1.WorkflowModeDAG,
						SubSteps: workflowv1alpha1.WorkflowModeStep,
					},
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb1",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb2",
								Type: "step-group",
							},
							SubSteps: []workflowv1alpha1.WorkflowStepBase{
								{
									Name:       "myweb2-sub1",
									Type:       "apply-component",
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
								},
								{
									Name:       "myweb2-sub2",
									Type:       "apply-component",
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb3"}`)},
								},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web3Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb3"}
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})

		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(BeNil())

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
		Expect(checkApp.Status.Workflow.Mode).Should(BeEquivalentTo(fmt.Sprintf("%s-%s", workflowv1alpha1.WorkflowModeDAG, workflowv1alpha1.WorkflowModeStep)))
	})

	It("application with mode in workflow step group", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-group-mode",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-group-mode"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-group-mode",
				Namespace: "app-with-group-mode",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb3",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Mode: &workflowv1alpha1.WorkflowExecuteMode{
						Steps: workflowv1alpha1.WorkflowModeDAG,
					},
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb1",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb2",
								Type: "step-group",
							},
							Mode: workflowv1alpha1.WorkflowModeStep,
							SubSteps: []workflowv1alpha1.WorkflowStepBase{
								{
									Name:       "myweb2-sub1",
									Type:       "apply-component",
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
								},
								{
									Name:       "myweb2-sub2",
									Type:       "apply-component",
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb3"}`)},
								},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web3Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb3"}
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})

		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(BeNil())

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
		Expect(checkApp.Status.Workflow.Mode).Should(BeEquivalentTo(fmt.Sprintf("%s-%s", workflowv1alpha1.WorkflowModeDAG, workflowv1alpha1.WorkflowModeDAG)))
	})

	It("application with sub steps", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-sub-steps",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-sub-steps"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-sub-steps",
				Namespace: "app-with-sub-steps",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       "myweb3",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb1",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb2",
								Type: "step-group",
							},
							SubSteps: []workflowv1alpha1.WorkflowStepBase{
								{
									Name:       "myweb2-sub1",
									Type:       "apply-component",
									DependsOn:  []string{"myweb2-sub2"},
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
								},
								{
									Name:       "myweb2-sub2",
									Type:       "apply-component",
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb3"}`)},
								},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})
		web3Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb3"}
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())

		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp = &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
	})

	It("application with array inputs", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-array-inputs",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-array-inputs"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-array-inputs",
				Namespace: "app-with-array-inputs",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
						Outputs: workflowv1alpha1.StepOutputs{
							{
								Name:      "output",
								ValueFrom: "context.name",
							},
						},
					},
					{
						Name:       "myweb2",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep"],"image":"busybox"}`)},
						Inputs: workflowv1alpha1.StepInputs{
							{
								From:         "output",
								ParameterKey: "cmd[1]",
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
	})

	It("application with timeout outputs in workflow", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-timeout-output",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-timeout-output"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-timeout-output",
				Namespace: "app-with-timeout-output",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:    "myweb1",
								Type:    "apply-component",
								Timeout: "1s",
								Outputs: workflowv1alpha1.StepOutputs{
									{
										Name:      "output",
										ValueFrom: "context.name",
									},
								},
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb2",
								Inputs: workflowv1alpha1.StepInputs{
									{
										From:         "output",
										ParameterKey: "",
									},
								},
								If:         `inputs.output == "app-with-timeout-output"`,
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		time.Sleep(time.Second)
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowFailed))
	})

	It("application with skip outputs in workflow", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-skip-output",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-skip-output"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-skip-output",
				Namespace: "app-with-skip-output",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb1",
								Type: "apply-component",
								If:   "false",
								Outputs: workflowv1alpha1.StepOutputs{
									{
										Name:      "output",
										ValueFrom: "context.name",
									},
								},
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb2",
								Inputs: workflowv1alpha1.StepInputs{
									{
										From:         "output",
										ParameterKey: "",
									},
								},
								If:         `inputs.output == "app-with-timeout-output"`,
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Steps[0].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseSkipped))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseSkipped))
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
	})

	It("application with invalid inputs in workflow", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-invalid-input",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-invalid-input"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-invalid-input",
				Namespace: "app-with-invalid-input",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb1",
								Type: "apply-component",
								Outputs: workflowv1alpha1.StepOutputs{
									{
										Name:      "output",
										ValueFrom: "context.name",
									},
								},
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb2",
								Inputs: workflowv1alpha1.StepInputs{
									{
										From:         "invalid",
										ParameterKey: "",
									},
								},
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Steps[0].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseSucceeded))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhasePending))
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
	})

	It("application with error inputs in workflow", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-error-input",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-error-input"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-error-input",
				Namespace: "app-with-error-input",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb1",
								Type: "apply-component",
								Outputs: workflowv1alpha1.StepOutputs{
									{
										Name:      "output",
										ValueFrom: "context.namespace",
									},
								},
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb2",
								Inputs: workflowv1alpha1.StepInputs{
									{
										From:         "output",
										ParameterKey: "cmd",
									},
								},
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Steps[0].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseSucceeded))
		for i := 0; i < wfTypes.MaxWorkflowStepErrorRetryTimes-1; i++ {
			testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
			Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
			Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
			Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(""))
			Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseFailed))
		}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowFailed))
	})

	It("application with invalid inputs in workflow in dag mode", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-invalid-input-dag",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-invalid-input-dag"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-invalid-input-dag",
				Namespace: "app-with-invalid-input-dag",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Mode: &workflowv1alpha1.WorkflowExecuteMode{
						Steps: workflowv1alpha1.WorkflowModeDAG,
					},
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb1",
								Type: "apply-component",
								Outputs: workflowv1alpha1.StepOutputs{
									{
										Name:      "output",
										ValueFrom: "context.name",
									},
								},
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb2",
								Inputs: workflowv1alpha1.StepInputs{
									{
										From:         "invalid",
										ParameterKey: "",
									},
								},
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Steps[0].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhaseRunning))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(workflowv1alpha1.WorkflowStepPhasePending))
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
	})

	It("application with if always in workflow", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-if-always-workflow",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-if-always-workflow"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-if-always-workflow",
				Namespace: "app-with-if-always-workflow",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       "failed-step",
						Type:       "k8s-objects",
						Properties: &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"invalid","metadata":{"name":"test1"}}]}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "failed-step",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"failed-step"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb1",
								Type:       "apply-component",
								If:         "always",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb2",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		By("verify the first ten reconciles")
		for i := 0; i < wfTypes.MaxWorkflowStepErrorRetryTimes; i++ {
			testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		}

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowFailed))
	})

	It("application with if always in workflow sub steps", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-if-always-workflow-sub-steps",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-if-always-workflow-sub-steps"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-if-always-workflow-sub-steps",
				Namespace: "app-with-if-always-workflow-sub-steps",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb3",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       "failed-step",
						Type:       "k8s-objects",
						Properties: &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"invalid","metadata":{"name":"test1"}}]}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb1",
								Type: "step-group",
							},
							SubSteps: []workflowv1alpha1.WorkflowStepBase{
								{
									Name:       "myweb1-sub1",
									Type:       "apply-component",
									If:         "always",
									DependsOn:  []string{"myweb1-sub2"},
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
								},
								{
									Name:       "myweb1-sub2",
									Type:       "apply-component",
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"failed-step"}`)},
								},
								{
									Name:       "myweb1-sub3",
									Type:       "apply-component",
									DependsOn:  []string{"myweb1-sub1"},
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
								},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb2",
								Type:       "apply-component",
								If:         "always",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb3",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb3"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		By("verify the first ten reconciles")
		for i := 0; i < wfTypes.MaxWorkflowStepErrorRetryTimes; i++ {
			testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		}

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})
		web3Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb3"}
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})

		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowFailed))
	})

	It("application with if expressions in workflow", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-if-expressions",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-if-expressions"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-if-expressions",
				Namespace: "app-with-if-expressions",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:    "suspend",
								Type:    "suspend",
								Timeout: "1s",
								Outputs: workflowv1alpha1.StepOutputs{
									{
										Name:      "suspend_output",
										ValueFrom: "context.name",
									},
								},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb1",
								Type: "apply-component",
								Inputs: workflowv1alpha1.StepInputs{
									{
										From:         "suspend_output",
										ParameterKey: "",
									},
								},
								If:         `status.suspend.timeout && inputs.suspend_output == "app-with-if-expressions"`,
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb2",
								If:         "status.suspend.succeeded",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowSuspending))

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		time.Sleep(time.Second)
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowFailed))
	})

	It("application with if expressions in workflow sub steps", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-if-expressions-workflow-sub-steps",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-if-expressions-workflow-sub-steps"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-if-expressions-workflow-sub-steps",
				Namespace: "app-with-if-expressions-workflow-sub-steps",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb1-sub",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb3",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb1",
								Type: "step-group",
							},
							SubSteps: []workflowv1alpha1.WorkflowStepBase{
								{
									Name:       "myweb1_sub1",
									Type:       "apply-component",
									If:         "status.myweb1_sub2.timeout",
									DependsOn:  []string{"myweb1_sub2"},
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
								},
								{
									Name:       "myweb1_sub2",
									Type:       "suspend",
									Properties: &runtime.RawExtension{Raw: []byte(`{"duration":"1s"}`)},
									Outputs: workflowv1alpha1.StepOutputs{
										{
											Name:      "suspend_output",
											ValueFrom: "context.name",
										},
									},
								},
								{
									Name:      "myweb1_sub3",
									Type:      "apply-component",
									DependsOn: []string{"myweb1_sub1"},
									Inputs: workflowv1alpha1.StepInputs{
										{
											From:         "suspend_output",
											ParameterKey: "",
										},
									},
									If:         `status.myweb1_sub1.timeout || inputs.suspend_output == "app-with-if-expressions-workflow-sub-steps"`,
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1-sub"}`)},
								},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb2",
								Type:       "apply-component",
								If:         "status.myweb1.failed",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb3",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb3"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowSuspending))

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})
		web1SubKey := types.NamespacedName{Namespace: ns.Name, Name: "myweb1-sub"}
		Expect(k8sClient.Get(ctx, web1SubKey, expDeployment)).Should(util.NotFoundMatcher{})
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})
		web3Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb3"}
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})

		time.Sleep(time.Second)
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})
		Expect(k8sClient.Get(ctx, web1SubKey, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})

		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
	})

	It("application with timeout in workflow", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-timeout",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-timeout"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-timeout",
				Namespace: "app-with-timeout",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       "myweb3",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "timeout-step",
								Type:       "apply-component",
								Timeout:    "1s",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb2",
								Type:       "apply-component",
								If:         "always",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb3",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb3"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})
		web3Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb3"}
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})
		time.Sleep(time.Second)

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Steps[0].Reason).Should(Equal(wfTypes.StatusReasonTimeout))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(Equal(workflowv1alpha1.WorkflowStepPhaseSucceeded))
		Expect(checkApp.Status.Workflow.Steps[2].Reason).Should(Equal(wfTypes.StatusReasonSkip))
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowFailed))
	})

	It("application with timeout and suspend in workflow", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-timeout-suspend",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-timeout-suspend"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-timeout-suspend",
				Namespace: "app-with-timeout-suspend",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:    "timeout-step",
								Type:    "suspend",
								Timeout: "1s",
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb1",
								Type:       "apply-component",
								If:         "always",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb2",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowSuspending))

		time.Sleep(time.Second)
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Steps[0].Reason).Should(Equal(wfTypes.StatusReasonTimeout))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(Equal(workflowv1alpha1.WorkflowStepPhaseSucceeded))
		Expect(checkApp.Status.Workflow.Steps[2].Reason).Should(Equal(wfTypes.StatusReasonSkip))
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowFailed))
	})

	It("application with timeout in workflow sub steps", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-timeout-sub-steps",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-timeout-sub-steps"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-timeout-sub-steps",
				Namespace: "app-with-timeout-sub-steps",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb3",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb4",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "myweb1",
								Type: "step-group",
							},
							SubSteps: []workflowv1alpha1.WorkflowStepBase{
								{
									Name:       "myweb1-sub1",
									Type:       "apply-component",
									If:         "always",
									DependsOn:  []string{"myweb1-sub2"},
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
								},
								{
									Name:       "myweb1-sub2",
									Type:       "apply-component",
									Timeout:    "1s",
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
								},
								{
									Name:       "myweb1-sub3",
									Type:       "apply-component",
									DependsOn:  []string{"myweb1-sub1"},
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb3"}`)},
								},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb3",
								Type:       "apply-component",
								If:         "always",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb3"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb4",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb4"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})
		web3Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb3"}
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})
		web4Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb4"}
		Expect(k8sClient.Get(ctx, web4Key, expDeployment)).Should(util.NotFoundMatcher{})

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})
		Expect(k8sClient.Get(ctx, web4Key, expDeployment)).Should(util.NotFoundMatcher{})

		time.Sleep(time.Second)

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})
		Expect(k8sClient.Get(ctx, web4Key, expDeployment)).Should(util.NotFoundMatcher{})
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web4Key, expDeployment)).Should(util.NotFoundMatcher{})
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, web4Key, expDeployment)).Should(util.NotFoundMatcher{})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Steps[0].Reason).Should(Equal(wfTypes.StatusReasonTimeout))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(Equal(workflowv1alpha1.WorkflowStepPhaseSucceeded))
		Expect(checkApp.Status.Workflow.Steps[2].Reason).Should(Equal(wfTypes.StatusReasonSkip))
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowFailed))
	})

	It("application with timeout and suspend in workflow sub steps", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-timeout-suspend-sub-steps",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-timeout-suspend-sub-steps"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-timeout-suspend-sub-steps",
				Namespace: "app-with-timeout-suspend-sub-steps",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:    "group1",
								Type:    "step-group",
								Timeout: "1s",
							},
							SubSteps: []workflowv1alpha1.WorkflowStepBase{
								{
									Name: "suspend",
									Type: "suspend",
								},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "group2",
								If:   "always",
								Type: "step-group",
							},
							SubSteps: []workflowv1alpha1.WorkflowStepBase{
								{
									Name:    "sub-suspend",
									Type:    "suspend",
									Timeout: "1s",
								},
								{
									Name:       "myweb1",
									Type:       "apply-component",
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
								},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb2",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowSuspending))

		time.Sleep(time.Second)
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowSuspending))
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		time.Sleep(time.Second)
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Workflow.Steps[0].Reason).Should(Equal(wfTypes.StatusReasonTimeout))
		Expect(checkApp.Status.Workflow.Steps[1].Reason).Should(Equal(wfTypes.StatusReasonTimeout))
		Expect(checkApp.Status.Workflow.Steps[2].Reason).Should(Equal(wfTypes.StatusReasonSkip))
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowFailed))
	})

	It("application with wait suspend in workflow", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-wait-suspend",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-wait-suspend"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-wait-suspend",
				Namespace: "app-with-wait-suspend",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "suspend",
								Type:       "suspend",
								Properties: &runtime.RawExtension{Raw: []byte(`{"duration":"1s"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "myweb1",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
							},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowSuspending))

		time.Sleep(time.Second)
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
	})

	It("application with input/output run as dag workflow", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-input-output",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-input-output"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		appwithInputOutput := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-input-output",
				Namespace: "app-with-input-output",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep", "10"],"image":"busybox"}`)},
						Inputs: workflowv1alpha1.StepInputs{
							{
								From:         "message",
								ParameterKey: "properties.enemies",
							},
							{
								From:         "message",
								ParameterKey: "properties.lives",
							},
							{
								From:         "sleepTime",
								ParameterKey: "properties.cmd[1]",
							},
						},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
						Outputs: workflowv1alpha1.StepOutputs{
							{Name: "message", ValueFrom: "output.status.conditions[0].message+\",\"+outputs.gameconfig.data.lives"},
							{Name: "sleepTime", ValueFrom: "\"100\""},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), appwithInputOutput)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: appwithInputOutput.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())

		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		expDeployment.Status.Conditions = []v1.DeploymentCondition{{
			Message: "hello",
		}}
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		expDeployment = &v1.Deployment{}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())
		Expect(expDeployment.Spec.Template.Spec.Containers[0].Command).Should(BeEquivalentTo([]string{"sleep", "100"}))
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp = &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))

		checkCM := &corev1.ConfigMap{}
		cmKey := types.NamespacedName{
			Name:      "myweb1game-config",
			Namespace: ns.Name,
		}
		Expect(k8sClient.Get(ctx, cmKey, checkCM)).Should(BeNil())
		Expect(checkCM.Data["enemies"]).Should(BeEquivalentTo("hello,i am lives"))
		Expect(checkCM.Data["lives"]).Should(BeEquivalentTo("hello,i am lives"))
	})

	It("application with depends on run as dag workflow", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-depends-on",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-depends-on"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		appwithDependsOn := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-depends-on",
				Namespace: "app-with-depends-on",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						DependsOn:  []string{"myweb2"},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), appwithDependsOn)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: appwithDependsOn.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())

		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		expDeployment.Status.Conditions = []v1.DeploymentCondition{{
			Message: "hello",
		}}
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment = &v1.Deployment{}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp = &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
	})

	It("application with input/output and depends on", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-input-output-depends-on",
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())
		healthComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(cdDefWithHealthStatusYaml))
		Expect(json.Unmarshal(hCDefJson, healthComponentDef)).Should(BeNil())
		healthComponentDef.Name = "worker-with-health"
		healthComponentDef.Namespace = "app-with-input-output-depends-on"
		Expect(k8sClient.Create(ctx, healthComponentDef)).Should(BeNil())
		appwithInputOutputDependsOn := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-with-input-output-depends-on",
				Namespace: "app-with-input-output-depends-on",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
						DependsOn:  []string{"myweb2"},
						Inputs: workflowv1alpha1.StepInputs{
							{
								From:         "message",
								ParameterKey: "properties.enemies",
							},
							{
								From:         "message",
								ParameterKey: "properties.lives",
							},
						},
					},
					{
						Name:       "myweb2",
						Type:       "worker-with-health",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","lives": "i am lives","enemies": "empty"}`)},
						Outputs: workflowv1alpha1.StepOutputs{
							{Name: "message", ValueFrom: "output.status.conditions[0].message+\",\"+outputs.gameconfig.data.lives"},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), appwithInputOutputDependsOn)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: appwithInputOutputDependsOn.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())

		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		expDeployment.Status.Conditions = []v1.DeploymentCondition{{
			Message: "hello",
		}}
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment = &v1.Deployment{}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp = &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))

		checkCM := &corev1.ConfigMap{}
		cmKey := types.NamespacedName{
			Name:      "myweb1game-config",
			Namespace: ns.Name,
		}
		Expect(k8sClient.Get(ctx, cmKey, checkCM)).Should(BeNil())
		Expect(checkCM.Data["enemies"]).Should(BeEquivalentTo("hello,i am lives"))
		Expect(checkCM.Data["lives"]).Should(BeEquivalentTo("hello,i am lives"))
	})

	It("test application applied resource in workflow step status", func() {
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-applied-resources",
			},
		}
		Expect(k8sClient.Create(context.Background(), &ns)).Should(BeNil())

		webComponentDef := &v1beta1.ComponentDefinition{}
		hCDefJson, _ := yaml.YAMLToJSON([]byte(componentDefYaml))
		Expect(json.Unmarshal(hCDefJson, webComponentDef)).Should(BeNil())
		webComponentDef.Name = "web-worker"
		webComponentDef.Namespace = "app-applied-resources"
		Expect(k8sClient.Create(ctx, webComponentDef)).Should(BeNil())

		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-applied-resources",
				Namespace: "app-applied-resources",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myweb1",
						Type:       "web-worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
					{
						Name:       "myweb2",
						Type:       "web-worker",
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
					},
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
		Expect(checkApp.Status.AppliedResources).Should(BeEquivalentTo([]common.ClusterObjectReference{
			{
				Cluster: "",
				Creator: common.WorkflowResourceCreator,
				ObjectReference: corev1.ObjectReference{Kind: "Deployment",
					Namespace:  "app-applied-resources",
					Name:       "myweb1",
					APIVersion: "apps/v1",
				},
			},
			{
				Cluster: "",
				Creator: common.WorkflowResourceCreator,
				ObjectReference: corev1.ObjectReference{Kind: "Deployment",
					Namespace:  "app-applied-resources",
					Name:       "myweb2",
					APIVersion: "apps/v1",
				},
			},
		}))

		// make error
		checkApp.Spec.Components[0].Properties = nil
		Expect(k8sClient.Update(context.Background(), checkApp)).Should(BeNil())
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp = &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.AppliedResources).Should(BeEquivalentTo([]common.ClusterObjectReference{
			{
				Cluster: "",
				Creator: common.WorkflowResourceCreator,
				ObjectReference: corev1.ObjectReference{Kind: "Deployment",
					Namespace:  "app-applied-resources",
					Name:       "myweb2",
					APIVersion: "apps/v1",
				},
			},
		}))
		Expect(checkApp.Status.Conditions[len(checkApp.Status.Conditions)-1].Type).Should(BeEquivalentTo("Render"))

		checkApp.Spec.Components[0] = common.ApplicationComponent{
			Name:       "myweb-1",
			Type:       "web-worker",
			Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
		}
		Expect(k8sClient.Update(context.Background(), checkApp)).Should(BeNil())
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp = &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
		Expect(checkApp.Status.AppliedResources).Should(BeEquivalentTo([]common.ClusterObjectReference{
			{
				Cluster: "",
				Creator: common.WorkflowResourceCreator,
				ObjectReference: corev1.ObjectReference{Kind: "Deployment",
					Namespace:  "app-applied-resources",
					Name:       "myweb-1",
					APIVersion: "apps/v1",
				},
			},
			{
				Cluster: "",
				Creator: common.WorkflowResourceCreator,
				ObjectReference: corev1.ObjectReference{Kind: "Deployment",
					Namespace:  "app-applied-resources",
					Name:       "myweb2",
					APIVersion: "apps/v1",
				},
			},
		}))
	})

	It("app apply resource in parallel", func() {
		wfDef := &v1beta1.WorkflowStepDefinition{}
		wfDefJson, _ := yaml.YAMLToJSON([]byte(applyInParallelWorkflowDefinitionYaml))
		Expect(json.Unmarshal(wfDefJson, wfDef)).Should(BeNil())
		Expect(k8sClient.Create(ctx, wfDef.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-apply-in-parallel",
			},
		}
		app := appwithNoTrait.DeepCopy()
		app.Name = "vela-test-app"
		app.SetNamespace(ns.Name)
		app.Spec.Workflow = &v1beta1.Workflow{
			Steps: []workflowv1alpha1.WorkflowStep{{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name:       "apply-in-parallel",
					Type:       "apply-test",
					Properties: &runtime.RawExtension{Raw: []byte(`{"parallelism": 20}`)},
				},
			}},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		_, err := testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(err).Should(BeNil())

		deployList := new(v1.DeploymentList)
		Expect(k8sClient.List(ctx, deployList, client.InNamespace(app.Namespace))).Should(BeNil())
		Expect(len(deployList.Items)).Should(Equal(20))

		checkApp := new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(Succeed())
		rt := new(v1beta1.ResourceTracker)
		expectRTName := fmt.Sprintf("%s-%s", checkApp.Status.LatestRevision.Name, checkApp.GetNamespace())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: expectRTName}, rt)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		Expect(len(rt.Spec.ManagedResources)).Should(Equal(20))
	})

	It("test controller requirement", func() {

		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-controller-requirement",
			},
		}
		Expect(k8sClient.Create(context.Background(), &ns)).Should(BeNil())

		appWithoutCtrlReq := appwithNoTrait.DeepCopy()
		appWithoutCtrlReq.SetNamespace(ns.Name)
		appWithoutCtrlReq.SetName("app-no-ctrl-req")
		Expect(k8sClient.Create(context.Background(), appWithoutCtrlReq)).Should(BeNil())

		appWithCtrlReqV1 := appwithNoTrait.DeepCopy()
		appWithCtrlReqV1.SetNamespace(ns.Name)
		appWithCtrlReqV1.SetName("app-with-ctrl-v1")
		appWithCtrlReqV1.Annotations = map[string]string{
			oam.AnnotationControllerRequirement: "v1",
		}
		Expect(k8sClient.Create(context.Background(), appWithCtrlReqV1)).Should(BeNil())

		appWithCtrlReqV2 := appwithNoTrait.DeepCopy()
		appWithCtrlReqV2.SetNamespace(ns.Name)
		appWithCtrlReqV2.SetName("app-with-ctrl-v2")
		appWithCtrlReqV2.Annotations = map[string]string{
			oam.AnnotationControllerRequirement: "v2",
		}
		Expect(k8sClient.Create(context.Background(), appWithCtrlReqV2)).Should(BeNil())

		v1OREmptyReconciler := *reconciler
		v1OREmptyReconciler.ignoreAppNoCtrlReq = false
		v1OREmptyReconciler.controllerVersion = "v1"

		v2OnlyReconciler := *reconciler
		v2OnlyReconciler.ignoreAppNoCtrlReq = true
		v2OnlyReconciler.controllerVersion = "v2"

		check := func(r reconcile.Reconciler, app *v1beta1.Application, do bool) {
			testutil.ReconcileOnceAfterFinalizer(r, reconcile.Request{NamespacedName: client.ObjectKey{
				Name:      app.Name,
				Namespace: app.Namespace,
			}})
			checkApp := &v1beta1.Application{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKey{
				Name:      app.Name,
				Namespace: app.Namespace,
			}, checkApp)).Should(BeNil())

			if do {
				Expect(checkApp.Annotations[oam.AnnotationKubeVelaVersion]).ShouldNot(BeEmpty())
			} else {
				if checkApp.Annotations == nil {
					return
				}
				Expect(checkApp.Annotations[oam.AnnotationKubeVelaVersion]).Should(BeEmpty())
			}
		}

		check(&v2OnlyReconciler, appWithoutCtrlReq, false)
		check(&v2OnlyReconciler, appWithCtrlReqV1, false)
		check(&v1OREmptyReconciler, appWithCtrlReqV2, false)

		check(&v1OREmptyReconciler, appWithoutCtrlReq, true)
		check(&v1OREmptyReconciler, appWithCtrlReqV1, true)
		check(&v2OnlyReconciler, appWithCtrlReqV2, true)
	})

	It("app with env and storage will create application", func() {

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-env-storage",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		appWithStorage.SetNamespace(ns.Name)
		app := appWithStorage.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App running successfully")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())

		By("Check affiliated resource tracker is created")
		expectRTName := fmt.Sprintf("%s-%s", appRevision.GetName(), appRevision.GetNamespace())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: expectRTName}, &v1beta1.ResourceTracker{})
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Check AppRevision Created with the expected workload spec")
		appRev := &v1beta1.ApplicationRevision{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: app.Name + "-v1", Namespace: app.GetNamespace()}, appRev)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("test application with trait-storage-secret-mountPath will be optional ", func() {

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-mountpath",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		appWithMountPath.SetNamespace(ns.Name)
		app := appWithMountPath.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App running successfully")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())

		By("Check affiliated resource tracker is created")
		expectRTName := fmt.Sprintf("%s-%s", appRevision.GetName(), appRevision.GetNamespace())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: expectRTName}, &v1beta1.ResourceTracker{})
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Check AppRevision Created with the expected workload spec")
		appRev := &v1beta1.ApplicationRevision{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: app.Name + "-v1", Namespace: app.GetNamespace()}, appRev)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Check secret Created with the expected trait-storage spec")
		secret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: ns.Name,
			Name:      app.Spec.Components[0].Name + "-secret",
		}, secret)).Should(BeNil())

		By("Check configMap Created with the expected trait-storage spec")
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: ns.Name,
			Name:      app.Spec.Components[0].Name + "-cm",
		}, cm)).Should(BeNil())

		Expect(k8sClient.Delete(ctx, cm)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, secret)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("test application with trait-gateway support https protocol", func() {

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-trait-gateway-https",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		appWithHttpsGateway.SetNamespace(ns.Name)
		app := appWithHttpsGateway.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App running successfully")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())

		By("Check affiliated resource tracker is created")
		expectRTName := fmt.Sprintf("%s-%s", appRevision.GetName(), appRevision.GetNamespace())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: expectRTName}, &v1beta1.ResourceTracker{})
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Check AppRevision Created with the expected workload spec")
		appRev := &v1beta1.ApplicationRevision{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: app.Name + "-v1", Namespace: app.GetNamespace()}, appRev)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Check ingress Created with the expected trait-gateway spec")
		ingress := &networkingv1.Ingress{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: ns.Name,
			Name:      app.Spec.Components[0].Name,
		}, ingress)).Should(BeNil())
		Expect(len(ingress.Spec.TLS) > 0).Should(BeTrue())
		Expect(ingress.Spec.TLS[0].SecretName).ShouldNot(BeNil())
		Expect(ingress.Spec.TLS[0].SecretName).ShouldNot(BeEmpty())
		Expect(len(ingress.Spec.TLS[0].Hosts) > 0).Should(BeTrue())
		Expect(ingress.Spec.TLS[0].Hosts[0]).ShouldNot(BeEmpty())
		Expect(k8sClient.Delete(ctx, ingress)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("test application with multi-mountToEnv will create application", func() {

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-mount-to-envs",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		appWithMountToEnvs.SetNamespace(ns.Name)
		app := appWithMountToEnvs.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App running successfully")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())
		By("Check affiliated resource tracker is created")
		expectRTName := fmt.Sprintf("%s-%s", appRevision.GetName(), appRevision.GetNamespace())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: expectRTName}, &v1beta1.ResourceTracker{})
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Check AppRevision Created with the expected workload spec")
		appRev := &v1beta1.ApplicationRevision{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: app.Name + "-v1", Namespace: app.GetNamespace()}, appRev)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Check secret Created with the expected trait-storage spec")
		secret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: ns.Name,
			Name:      app.Spec.Components[0].Name + "-secret",
		}, secret)).Should(BeNil())

		By("Check configMap Created with the expected trait-storage spec")
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: ns.Name,
			Name:      app.Spec.Components[0].Name + "-cm",
		}, cm)).Should(BeNil())

		Expect(k8sClient.Delete(ctx, cm)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, secret)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app with debug policy", func() {
		app := &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-debug",
				Namespace: "default",
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name:       "myworker",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\",\"env\":[{\"name\":\"firstKey\",\"value\":\"firstValue\"}]}")},
					},
					{
						Name:       "myworker2",
						Type:       "worker",
						Properties: &runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\",\"env\":[{\"name\":\"firstKey\",\"value\":\"firstValue\"}]}")},
					},
				},
				Policies: []v1beta1.AppPolicy{
					{
						Type: "debug",
						Name: "debug",
					},
				},
				Workflow: &v1beta1.Workflow{
					Steps: []workflowv1alpha1.WorkflowStep{
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name:       "step1",
								Type:       "apply-component",
								Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myworker"}`)},
							},
						},
						{
							WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
								Name: "step2",
								Type: "step-group",
							},
							SubSteps: []workflowv1alpha1.WorkflowStepBase{
								{
									Name:       "step2-sub1",
									Type:       "apply-component",
									Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myworker2"}`)},
								},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App running successfully")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		By("Check debug Config Map is created")
		debugCM := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      debug.GenerateContextName(app.Name, curApp.Status.Workflow.Steps[0].ID, string(app.UID)),
			Namespace: "default",
		}, debugCM)).Should(BeNil())
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      debug.GenerateContextName(app.Name, curApp.Status.Workflow.Steps[1].SubStepsStatus[0].ID, string(app.UID)),
			Namespace: "default",
		}, debugCM)).Should(BeNil())

		By("Update the application to update the debug Config Map")
		app.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\",\"env\":[{\"name\":\"firstKey\",\"value\":\"updateValue\"}]}")}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		updatedCM := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      debug.GenerateContextName(app.Name, curApp.Status.Workflow.Steps[0].ID, string(app.UID)),
			Namespace: "default",
		}, updatedCM)).Should(BeNil())

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("test application with controlPlaneOnly trait ", func() {

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-controlplaneonly",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		appWithControlPlaneOnly.SetNamespace(ns.Name)
		app := appWithControlPlaneOnly.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App running successfully")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())

		By("Check affiliated resource tracker is created")
		expectRTName := fmt.Sprintf("%s-%s", appRevision.GetName(), appRevision.GetNamespace())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: expectRTName}, &v1beta1.ResourceTracker{})
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Check AppRevision Created with the expected workload spec")
		appRev := &v1beta1.ApplicationRevision{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: app.Name + "-v1", Namespace: app.GetNamespace()}, appRev)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Check secret Created with the expected trait-storage spec")
		hpa := &autoscalingv1.HorizontalPodAutoscaler{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.GetNamespace(),
			Name:      app.Spec.Components[0].Name,
		}, hpa)).Should(BeNil())

		Expect(k8sClient.Delete(ctx, hpa)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("test application with apply-once policy ", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-apply-once",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		appWithApplyOnce.SetNamespace(ns.Name)
		app := appWithApplyOnce.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App running successfully")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())

		By("Check affiliated resource tracker is created")
		expectRTName := fmt.Sprintf("%s-%s", appRevision.GetName(), appRevision.GetNamespace())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: expectRTName}, &v1beta1.ResourceTracker{})
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Check AppRevision Created with the expected workload spec")
		appRev := &v1beta1.ApplicationRevision{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: app.Name + "-v1", Namespace: app.GetNamespace()}, appRev)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Check secret Created with the expected trait-storage spec")
		deployment := &v1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.GetNamespace(),
			Name:      app.Spec.Components[0].Name,
		}, deployment)).Should(BeNil())
		targetReplicas := int32(5)
		deployment.Spec.Replicas = &targetReplicas
		Expect(k8sClient.Update(ctx, deployment)).Should(BeNil())

		newDeployment := &v1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.GetNamespace(),
			Name:      app.Spec.Components[0].Name,
		}, newDeployment)).Should(BeNil())
		Expect(*newDeployment.Spec.Replicas).Should(Equal(targetReplicas))
		Expect(k8sClient.Delete(ctx, newDeployment)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("test application with healthProbe which use https", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-httpshealthprobe",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		appWithHttpsHealthProbe.SetNamespace(ns.Name)
		app := appWithHttpsHealthProbe.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App running successfully")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())

		By("Check affiliated resource tracker is created")
		expectRTName := fmt.Sprintf("%s-%s", appRevision.GetName(), appRevision.GetNamespace())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: expectRTName}, &v1beta1.ResourceTracker{})
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Check AppRevision Created with the expected workload spec")
		appRev := &v1beta1.ApplicationRevision{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: app.Name + "-v1", Namespace: app.GetNamespace()}, appRev)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("test application with pod affinity will create application", func() {

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-affinity",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		appWithAffinity.SetNamespace(ns.Name)
		app := appWithAffinity.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App running successfully")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		appRevision := &v1beta1.ApplicationRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())
		By("Check affiliated resource tracker is created")
		expectRTName := fmt.Sprintf("%s-%s", appRevision.GetName(), appRevision.GetNamespace())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: expectRTName}, &v1beta1.ResourceTracker{})
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Check AppRevision Created with the expected workload spec")
		appRev := &v1beta1.ApplicationRevision{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: app.Name + "-v1", Namespace: app.GetNamespace()}, appRev)
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("test cue panic", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cue-panic",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		appWithMountToEnvs.SetNamespace(ns.Name)
		app := appWithMountToEnvs.DeepCopy()
		app.Spec.Components[0].Traits = []common.ApplicationTrait{
			{
				Type:       "panic",
				Properties: &runtime.RawExtension{Raw: []byte("{\"configMap\": [{\"name\": \"myweb-cm\"}]}")},
			},
		}
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		// should not panic anymore, refer to https://github.com/cue-lang/cue/issues/1828
		// Expect(curApp.Status.Phase).Should(Equal(common.ApplicationWorkflowFailed))
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))
	})

	It("test application with healthy and PreDispatch trait", func() {
		defer featuregatetesting.SetFeatureGateDuringTest(&testing.T{}, utilfeature.DefaultFeatureGate, features.MultiStageComponentApply, true)()
		By("create the new namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-pre-dispatch-healthy",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		appWithPreDispatch := appwithNoTrait.DeepCopy()
		appWithPreDispatch.Spec.Components[0].Name = "comp-with-pre-dispatch-trait"
		appWithPreDispatch.Spec.Components[0].Traits = []common.ApplicationTrait{
			{
				Type:       "storage-pre-dispatch",
				Properties: &runtime.RawExtension{Raw: []byte("{\"configMap\":[{\"name\":\"pre-dispatch-cm\",\"mountPath\":\"/test/mount/cm\",\"data\":{\"firstKey\":\"firstValue\"}}]}")},
			},
		}
		appWithPreDispatch.Name = "app-with-pre-dispatch"
		appWithPreDispatch.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, appWithPreDispatch)).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      appWithPreDispatch.Name,
			Namespace: appWithPreDispatch.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App running successfully")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		By("Check Manifests Created")
		expConfigMap := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      "pre-dispatch-cm",
			Namespace: appWithPreDispatch.Namespace,
		}, expConfigMap)).Should(BeNil())

		expDeployment := &v1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      appWithPreDispatch.Spec.Components[0].Name,
			Namespace: appWithPreDispatch.Namespace,
		}, expDeployment)).Should(BeNil())

		Expect(k8sClient.Delete(ctx, appWithPreDispatch)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, ns)).Should(BeNil())
	})

	It("test application with unhealthy and PreDispatch trait", func() {
		defer featuregatetesting.SetFeatureGateDuringTest(&testing.T{}, utilfeature.DefaultFeatureGate, features.MultiStageComponentApply, true)()
		By("create the new namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-pre-dispatch-unhealthy",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		appWithPreDispatch := appwithNoTrait.DeepCopy()
		appWithPreDispatch.Spec.Components[0].Name = "comp-with-pre-dispatch-trait"
		appWithPreDispatch.Spec.Components[0].Traits = []common.ApplicationTrait{
			{
				Type:       "storage-pre-dispatch-unhealthy",
				Properties: &runtime.RawExtension{Raw: []byte("{\"configMap\":[{\"name\":\"pre-dispatch-unhealthy-cm\",\"mountPath\":\"/test/mount/cm\",\"data\":{\"firstKey\":\"firstValue\"}}]}")},
			},
		}
		appWithPreDispatch.Name = "app-with-pre-dispatch-unhealthy"
		appWithPreDispatch.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, appWithPreDispatch)).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      appWithPreDispatch.Name,
			Namespace: appWithPreDispatch.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check the Application status")
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunningWorkflow))

		By("Check Manifests Created")
		expConfigMap := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      "pre-dispatch-unhealthy-cm",
			Namespace: appWithPreDispatch.Namespace,
		}, expConfigMap)).Should(BeNil())

		expDeployment := &v1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      appWithPreDispatch.Spec.Components[0].Name,
			Namespace: appWithPreDispatch.Namespace,
		}, expDeployment)).Should(util.NotFoundMatcher{})

		Expect(k8sClient.Delete(ctx, appWithPreDispatch)).Should(BeNil())
		Expect(k8sClient.Delete(ctx, ns)).Should(BeNil())
	})
})

const (
	scopeDefYaml = `apiVersion: core.oam.dev/v1beta1
kind: ScopeDefinition
metadata:
  name: healthscopes.core.oam.dev
  namespace: vela-system
spec:
  workloadRefsPath: spec.workloadRefs
  allowComponentOverlap: true
  definitionRef:
    name: healthscopes.core.oam.dev`

	componentDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  extension:
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

              selector:
                  matchLabels:
                      "app.oam.dev/component": context.name
          }
      }

      parameter: {
          // +usage=Which image would you like to use for your service
          // +short=i
          image: string

          cmd?: [...string]
      }
`

	unhealthyComponentDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: unhealthy-worker
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  extension:
    template: |
      output: {
          apiVersion: "apps/v1"
          kind:       "Deployment"
          metadata: {
              annotations: {
                  if context["config"] != _|_ {
                      for _, v in context.config {
                          "\(v.name)" : v.value
                      }
                  }
              }
          }
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

              selector:
                  matchLabels:
                      "app.oam.dev/component": context.name
          }
      }

      parameter: {
          // +usage=Which image would you like to use for your service
          // +short=i
          image: string
          cmd?: [...string]
      }
  status:
    healthPolicy: |-
      isHealth: false
`

	wDImportYaml = `
apiVersion: core.oam.dev/v1beta1
kind: WorkloadDefinition
metadata:
  name: worker-import
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  definitionRef:
    name: deployments.apps
  extension:
    template: |
      import (
          "k8s.io/apps/v1"
          appsv1 "kube/apps/v1"
      )
      output: v1.#Deployment & appsv1.#Deployment & {
          metadata: {
              annotations: {
                  if context["config"] != _|_ {
                      for _, v in context.config {
                          "\(v.name)" : v.value
                      }
                  }
              }
          }
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

              selector:
                  matchLabels:
                      "app.oam.dev/component": context.name
          }
      }

      parameter: {
          // +usage=Which image would you like to use for your service
          // +short=i
          image: string

          cmd?: [...string]
      }
`

	tdImportedYaml = `apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: ingress-import
  namespace: vela-system
spec:
  appliesToWorkloads:
    - "*"
  schematic:
    cue:
      template: |
        import (
        	kubev1 "k8s.io/core/v1"
        	network "k8s.io/networking/v1"
        )

        parameter: {
        	domain: string
        	http: [string]: int
        }

        outputs: {
        service: kubev1.#Service
        ingress: network.#Ingress
        }

        // trait template can have multiple outputs in one trait
        outputs: service: {
        	metadata:
        		name: context.name
        	spec: {
        		selector:
        			"app.oam.dev/component": context.name
        		ports: [
        			for k, v in parameter.http {
        				port:       v
        				targetPort: v
        			},
        		]
        	}
        }

        outputs: ingress: {
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
        }`

	webComponentDefYaml = `apiVersion: core.oam.dev/v1alpha2
kind: ComponentDefinition
metadata:
  name: webserver
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "webserver was composed by deployment and service"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  extension:
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
      				}]
      		}
      		}
      	}
      }
      // workload can have extra object composition by using 'outputs' keyword
      outputs: service: {
      	apiVersion: "v1"
      	kind:       "Service"
      	spec: {
      		selector: {
      			"app.oam.dev/component": context.name
      		}
      		ports: [
      			{
      				port:       parameter.port
      				targetPort: parameter.port
      			},
      		]
      	}
      }
      parameter: {
      	image: string
      	cmd?: [...string]
      	port: *80 | int
      	env?: [...{
      		name:   string
      		value?: string
      		valueFrom?: {
      			secretKeyRef: {
      				name: string
      				key:  string
      			}
      		}
      	}]
      	cpu?: string
      }

`
	componentDefWithHealthYaml = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  extension:
    healthPolicy: |
      isHealth: context.output.status.readyReplicas == context.output.status.replicas 
    template: |
      output: {
          apiVersion: "apps/v1"
          kind:       "Deployment"
          metadata: {
              annotations: {
                  if context["config"] != _|_ {
                      for _, v in context.config {
                          "\(v.name)" : v.value
                      }
                  }
              }
          }
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

              selector:
                  matchLabels:
                      "app.oam.dev/component": context.name
          }
      }

      parameter: {
          // +usage=Which image would you like to use for your service
          // +short=i
          image: string
          cmd?: [...string]
      }
`

	cdDefWithHealthStatusYaml = `apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: nworker
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic."
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  status:
    healthPolicy: |
      isHealth: (context.output.status.readyReplicas > 0) && (context.output.status.readyReplicas == context.output.status.replicas)
    customStatus: |-
      message: "type: " + context.output.spec.template.spec.containers[0].image + ",\t enemies:" + context.outputs.gameconfig.data.enemies
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
        					envFrom: [{
        						configMapRef: name: context.name + "game-config"
        					}]
        					if parameter["cmd"] != _|_ {
        						command: parameter.cmd
        					}
        				}]
        			}
        		}
        	}
        }

        outputs: gameconfig: {
        	apiVersion: "v1"
        	kind:       "ConfigMap"
        	metadata: {
        		name: context.name + "game-config"
        	}
        	data: {
        		enemies: parameter.enemies
        		lives:   parameter.lives
        	}
        }

        parameter: {
        	// +usage=Which image would you like to use for your service
        	// +short=i
        	image: string
        	// +usage=Commands to run in the container
        	cmd?: [...string]
        	lives:   string
        	enemies: string
        }
`
	workloadDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: WorkloadDefinition
metadata:
  name: task
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Describes jobs that run code or a script to completion."
spec:
  definitionRef:
    name: jobs.batch
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

	tDDefWithHealthStatusYaml = `apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: ingress
  namespace: vela-system
spec:
  status:
    customStatus: |-
      message: "type: "+ context.outputs.service.spec.type +",\t clusterIP:"+ context.outputs.service.spec.clusterIP+",\t ports:"+ "\(context.outputs.service.spec.ports[0].port)"+",\t domain"+context.outputs.ingress.spec.rules[0].host
    healthPolicy: |
      isHealth: len(context.outputs.service.spec.clusterIP) > 0
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
        	spec: {
        		selector:
        			app: context.name
        		ports: [
        			for k, v in parameter.http {
        				port:       v
        				targetPort: v
        			},
        		]
        	}
        }
        outputs: ingress: {
        	apiVersion: "networking.k8s.io/v1"
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
                                pathType: "Prefix"
        						backend: {
        							service: {
                                        name: context.name
                                        port: {
                                            number: v
                                        }
                                    }
        						}
        					},
        				]
        			}
        		}]
        	}
        }
`
	workloadWithContextRevision = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker-revision
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  extension:
    healthPolicy: |
      isHealth: context.output.status.readyReplicas == context.output.status.replicas 
    template: |
      output: {
          apiVersion: "apps/v1"
          kind:       "Deployment"
          metadata: {
              annotations: {
                  if context["config"] != _|_ {
                      for _, v in context.config {
                          "\(v.name)" : v.value
                      }
                  }
              }
          }
          spec: {
              selector: matchLabels: {
                  "app.oam.dev/component": context.name
              }
              template: {
                  metadata: labels: {
                      "app.oam.dev/component": context.name
                      "app.oam.dev/revision": context.revision
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

              selector:
                  matchLabels:
                      "app.oam.dev/component": context.name
          }
      }

      parameter: {
          // +usage=Which image would you like to use for your service
          // +short=i
          image: string

          cmd?: [...string]
      }`

	shareFsTraitDefinition = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: share-fs
  namespace: default
spec:
  schematic:
    cue:
      template: |
        outputs: pv: {
        	apiVersion: "v1"
        	kind:       "PersistentVolume"
        	metadata: {
        		name:      context.name
        	}
        	spec: {
        		accessModes: ["ReadWriteMany"]
        		capacity: storage: "999Gi"
        		persistentVolumeReclaimPolicy: "Retain"
        		csi: {
        			driver: "nasplugin.csi.alibabacloud.com"
        			volumeAttributes: {
        				host: nasConn.MountTargetDomain
        				path: "/"
        				vers: "3.0"
        			}
        			volumeHandle: context.name
        		}
        	}
        }
        outputs: pvc: {
        	apiVersion: "v1"
        	kind:       "PersistentVolumeClaim"
        	metadata: {
        		name:      parameter.pvcName
        	}
        	spec: {
        		accessModes: ["ReadWriteMany"]
        		resources: {
        			requests: {
        				storage: "999Gi"
        			}
        		}
        		volumeName: context.name
        	}
        }
        parameter: {
        	pvcName: string
        	// +insertSecretTo=nasConn
        	nasSecret: string
        }
        nasConn: {
        	MountTargetDomain: string
        }
`
	rolloutTraitDefinition = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: rollout
  namespace: default
spec:
  manageWorkload: true
  skipRevisionAffect: true
  schematic:
    cue:
      template: |
        outputs: rollout: {
        	apiVersion: "standard.oam.dev/v1alpha1"
        	kind:       "Rollout"
        	metadata: {
        		name:  context.name
                namespace: context.namespace
        	}
        	spec: {
                   targetRevisionName: parameter.targetRevision
                   componentName: "myweb1"
                   rolloutPlan: {
                   	rolloutStrategy: "IncreaseFirst"
                    rolloutBatches:[
                    	{ replicas: 3}]    
                    targetSize: 5
                   }
        		 }
        	}

         parameter: {
             targetRevision: *context.revision|string
         }
`
	applyCompWfStepDefinition = `
apiVersion: core.oam.dev/v1beta1
kind: WorkflowStepDefinition
metadata:
  annotations:
    definition.oam.dev/description: Apply components and traits for your workflow steps
  name: apply-component
  namespace: vela-system
spec:
  schematic:
    cue:
      template: |
        import (
        	"vela/op"
        )

        // apply components and traits
        apply: op.#ApplyComponent & {
        	component: parameter.component
        }
        parameter: {
        	// +usage=Declare the name of the component
        	component: string
        }
`

	k8sObjectsComponentDefinitionYaml = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  annotations:
    definition.oam.dev/description: K8s-objects allow users to specify raw K8s objects in properties
  name: k8s-objects
  namespace: vela-system
spec:
  schematic:
    cue:
      template: |
        output: parameter.objects[0]
        outputs: {
          for i, v in parameter.objects {
            if i > 0 {
              "objects-\(i)": v
            }
          }
        }
        parameter: objects: [...{}]
`
	applyInParallelWorkflowDefinitionYaml = `
apiVersion: core.oam.dev/v1beta1
kind: WorkflowStepDefinition
metadata:
  name: apply-test
  namespace: vela-system
spec:
  schematic:
    cue:
      template: |
        import (
                "vela/op"
                "list"
        )

        components:      op.#LoadInOrder & {}
        targetComponent: components.value[0]
        resources:       op.#RenderComponent & {
                value: targetComponent
        }
        workload:       resources.output
        arr:            list.Range(0, parameter.parallelism, 1)
        patchWorkloads: op.#Steps & {
                for idx in arr {
                        "\(idx)": op.#PatchK8sObject & {
                                value: workload
                                patch: {
                                        // +patchStrategy=retainKeys
                                        metadata: name: "\(targetComponent.name)-\(idx)"
                                }
                        }
                }
        }
        workloads: [ for patchResult in patchWorkloads {patchResult.result}]
        apply: op.#ApplyInParallel & {
                value: workloads
        }
        parameter: parallelism: int

`
)
