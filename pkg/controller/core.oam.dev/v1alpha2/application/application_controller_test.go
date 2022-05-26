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
	"net"
	"net/http"
	"net/http/httptest"
	"time"

	"k8s.io/utils/pointer"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	v1beta12 "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	stdv1alpha1 "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/testutil"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/workflow"
	"github.com/oam-dev/kubevela/pkg/workflow/debug"
	"github.com/oam-dev/kubevela/pkg/workflow/tasks/custom"
)

// TODO: Refactor the tests to not copy and paste duplicated code 10 times
var _ = Describe("Test Application Controller", func() {
	ctx := context.TODO()
	appwithConfig := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-with-config",
			Namespace: "app-with-config",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "myweb1",
					Type:       "worker",
					Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","config":"myconfig"}`)},
				},
			},
		},
	}
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

	appImportPkg := &v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-import-pkg",
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       "myweb",
					Type:       "worker-import",
					Properties: &runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\"}")},
					Traits: []common.ApplicationTrait{
						{
							Type:       "ingress-import",
							Properties: &runtime.RawExtension{Raw: []byte("{\"http\":{\"/\":80},\"domain\":\"abc.com\"}")},
						},
					},
				},
			},
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
	expectScalerTrait := func(compName, appName, appNs, traitName, traitNamespace string) unstructured.Unstructured {
		return unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "core.oam.dev/v1alpha2",
			"kind":       "ManualScalerTrait",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{"oam.dev/kubevela-version": "UNKNOWN"},
				"labels": map[string]interface{}{
					"trait.oam.dev/type":       "scaler",
					"app.oam.dev/component":    compName,
					"app.oam.dev/name":         appName,
					"app.oam.dev/namespace":    appNs,
					"app.oam.dev/appRevision":  appName + "-v1",
					"app.oam.dev/resourceType": "TRAIT",
					"trait.oam.dev/resource":   "scaler",
				},
				"name":      traitName,
				"namespace": traitNamespace,
			},
			"spec": map[string]interface{}{
				"replicaCount": int64(2),
				"workloadRef": map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"name":       compName,
				},
			},
		}}
	}

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
			Properties: &runtime.RawExtension{Raw: []byte("{\"env\":{\"thirdKey\":\"thirdValue\"}}")},
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
				},
			},
		},
	}
	appWithControlPlaneOnly.Spec.Components[0].Traits = []common.ApplicationTrait{
		{
			Type:       "hubcpuscaler",
			Properties: &runtime.RawExtension{Raw: []byte("{\"min\": 1,\"max\": 10,\"cpuPercent\": 60}")},
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
	importGateway := &v1alpha2.TraitDefinition{}
	importStorage := &v1alpha2.TraitDefinition{}

	importEnv := &v1alpha2.TraitDefinition{}

	importHubCpuScaler := &v1beta1.TraitDefinition{}

	importPodAffinity := &v1beta1.TraitDefinition{}

	webserverwd := &v1alpha2.ComponentDefinition{}
	webserverwdJson, _ := yaml.YAMLToJSON([]byte(webComponentDefYaml))

	td := &v1beta1.TraitDefinition{}
	tDDefJson, _ := yaml.YAMLToJSON([]byte(traitDefYaml))

	sd := &v1beta1.ScopeDefinition{}
	sdDefJson, _ := yaml.YAMLToJSON([]byte(scopeDefYaml))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "kubevela-app-with-config-myweb1-myconfig", Namespace: appwithConfig.Namespace},
		Data:       map[string]string{"c1": "v1", "c2": "v2"},
	}

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

		gatewayJson, gatewayErr := yaml.YAMLToJSON([]byte(gatewayYaml))
		Expect(gatewayErr).ShouldNot(HaveOccurred())
		Expect(json.Unmarshal(gatewayJson, importGateway)).Should(BeNil())
		Expect(k8sClient.Create(ctx, importGateway.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		storageJson, storageErr := yaml.YAMLToJSON([]byte(storageYaml))
		Expect(storageErr).ShouldNot(HaveOccurred())
		Expect(json.Unmarshal(storageJson, importStorage)).Should(BeNil())
		Expect(k8sClient.Create(ctx, importStorage.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		envJson, envErr := yaml.YAMLToJSON([]byte(envYaml))
		Expect(envErr).ShouldNot(HaveOccurred())
		Expect(json.Unmarshal(envJson, importEnv)).Should(BeNil())
		Expect(k8sClient.Create(ctx, importEnv.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		hubCpuScalerJson, hubCpuScalerErr := yaml.YAMLToJSON([]byte(hubCpuScalerYaml))
		Expect(hubCpuScalerErr).ShouldNot(HaveOccurred())
		Expect(json.Unmarshal(hubCpuScalerJson, importHubCpuScaler)).Should(BeNil())
		Expect(k8sClient.Create(ctx, importHubCpuScaler.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		affinityJson, podAffinityErr := yaml.YAMLToJSON([]byte(affinityYaml))
		Expect(podAffinityErr).ShouldNot(HaveOccurred())
		Expect(json.Unmarshal(affinityJson, importPodAffinity)).Should(BeNil())
		Expect(k8sClient.Create(ctx, importPodAffinity.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		Expect(json.Unmarshal(tDDefJson, td)).Should(BeNil())
		Expect(k8sClient.Create(ctx, td.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

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

	It("app-with-trait will create workload and trait", func() {

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-trait",
			},
		}
		appWithTrait.SetNamespace(ns.Name)
		expDeployment := getExpDeployment("myweb3", appWithTrait)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		app := appWithTrait.DeepCopy()
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

		Expect(len(comp.Traits) > 0).Should(BeTrue())
		gotTrait := comp.Traits[0]
		Expect(cmp.Diff(*gotTrait, expectScalerTrait("myweb3", app.Name, app.Namespace, gotTrait.GetName(), gotTrait.GetNamespace()))).Should(BeEmpty())

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app-with-composedworkload-trait will create workload and trait", func() {
		compName := "myweb-composed-3"
		var appname = "app-with-composedworkload-trait"

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-composedworkload-trait",
			},
		}

		appWithComposedWorkload := appwithNoTrait.DeepCopy()
		appWithComposedWorkload.Spec.Components[0].Type = "webserver"
		appWithComposedWorkload.SetName(appname)
		appWithComposedWorkload.Spec.Components[0].Traits = []common.ApplicationTrait{
			{
				Type:       "scaler",
				Properties: &runtime.RawExtension{Raw: []byte(`{"replicas":2}`)},
			},
		}
		appWithComposedWorkload.Spec.Components[0].Name = compName
		appWithComposedWorkload.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		app := appWithComposedWorkload.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		expDeployment := getExpDeployment(compName, appWithComposedWorkload)
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
		af, err := appParser.GenerateAppFileFromRevision(appRevision)
		Expect(err).Should(BeNil())
		comps, err := af.GenerateComponentManifests()
		Expect(err).Should(BeNil())
		Expect(len(comps) > 0).Should(BeTrue())
		comp := comps[0]

		Expect(comp.StandardWorkload).ShouldNot(BeNil())
		gotDeploy := &v1.Deployment{}
		Expect(runtime.DefaultUnstructuredConverter.FromUnstructured(comp.StandardWorkload.Object, gotDeploy)).Should(Succeed())
		gotDeploy.Annotations = nil
		expDeployment.ObjectMeta.Labels["workload.oam.dev/type"] = "webserver"
		expDeployment.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{{ContainerPort: 80}}
		Expect(cmp.Diff(gotDeploy, expDeployment)).Should(BeEmpty())

		Expect(len(comp.Traits)).Should(BeEquivalentTo(2))
		gotTrait := comp.Traits[0]
		By("Check the first trait should be service")
		expectServiceTrait := unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":        "myweb-composed-3-auxiliaryworkload-6f9c9cd64c",
				"namespace":   "vela-test-with-composedworkload-trait",
				"annotations": map[string]interface{}{"oam.dev/kubevela-version": string("UNKNOWN")},
				"labels": map[string]interface{}{
					"trait.oam.dev/type":       "AuxiliaryWorkload",
					"app.oam.dev/name":         "app-with-composedworkload-trait",
					"app.oam.dev/namespace":    ns.Name,
					"app.oam.dev/appRevision":  "app-with-composedworkload-trait-v1",
					"app.oam.dev/component":    "myweb-composed-3",
					"trait.oam.dev/resource":   "service",
					"app.oam.dev/resourceType": "TRAIT",
				},
			},
			"spec": map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{"port": int64(80), "targetPort": int64(80)},
				},
				"selector": map[string]interface{}{
					"app.oam.dev/component": compName,
				},
			},
		}}
		Expect(cmp.Diff(expectServiceTrait, *gotTrait)).Should(BeEmpty())

		By("Check the second trait should be scaler")
		gotTrait = comp.Traits[1]
		Expect(cmp.Diff(expectScalerTrait("myweb-composed-3", app.Name, app.Namespace, gotTrait.GetName(), gotTrait.GetNamespace()), *gotTrait)).Should(BeEmpty())

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app-with-trait-and-scope will create workload, trait and scope", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-trait-scope",
			},
		}
		appWithTraitAndScope.SetNamespace(ns.Name)
		expDeployment := getExpDeployment("myweb4", appWithTraitAndScope)

		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		app := appWithTraitAndScope.DeepCopy()
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

		By("Check App scope status")
		scopes := curApp.Status.Services[0].Scopes
		Expect(len(scopes)).Should(BeEquivalentTo(1))
		Expect(scopes[0].APIVersion).Should(BeEquivalentTo("core.oam.dev/v1alpha2"))
		Expect(scopes[0].Kind).Should(BeEquivalentTo("HealthScope"))
		Expect(scopes[0].Name).Should(BeEquivalentTo("appWithTraitAndScope-default-health"))

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
		af, err := appParser.GenerateAppFileFromRevision(appRevision)
		Expect(err).Should(BeNil())
		comps, err := af.GenerateComponentManifests()
		Expect(err).Should(BeNil())
		Expect(len(comps) > 0).Should(BeTrue())
		comp := comps[0]
		Expect(len(comp.Traits) > 0).Should(BeTrue())
		gotTrait := comp.Traits[0]
		Expect(cmp.Diff(*gotTrait, expectScalerTrait("myweb4", app.Name, app.Namespace, gotTrait.GetName(), gotTrait.GetNamespace()))).Should(BeEmpty())

		Expect(len(comp.Scopes) > 0).Should(BeTrue())
		gotScope := comp.Scopes[0]
		Expect(*gotScope).Should(BeEquivalentTo(corev1.ObjectReference{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "HealthScope",
			Name:       "appWithTraitAndScope-default-health",
		}))

		Expect(comp.StandardWorkload).ShouldNot(BeNil())
		gotD := &v1.Deployment{}
		Expect(runtime.DefaultUnstructuredConverter.FromUnstructured(comp.StandardWorkload.Object, gotD)).Should(Succeed())
		gotD.Annotations = nil
		Expect(cmp.Diff(gotD, expDeployment)).Should(BeEmpty())

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app with two components and update", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-with-two-comps",
			},
		}
		appWithTwoComp.SetNamespace(ns.Name)
		expDeployment := getExpDeployment("myweb5", appWithTwoComp)

		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		app := appWithTwoComp.DeepCopy()
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		cm.SetNamespace(ns.Name)
		cm.SetName("kubevela-app-with-two-comp-myweb6-myconfig")
		Expect(k8sClient.Create(ctx, cm.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		curApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.ObservedGeneration).Should(BeZero())

		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App running successfully")
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.ObservedGeneration).Should(Equal(curApp.Generation))
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

		af, err := appParser.GenerateAppFileFromRevision(appRevision)
		Expect(err).Should(BeNil())
		comps, err := af.GenerateComponentManifests()
		Expect(err).Should(BeNil())
		Expect(len(comps) > 0).Should(BeTrue())
		comp1 := comps[0]
		Expect(len(comp1.Traits) > 0).Should(BeTrue())
		gotTrait := comp1.Traits[0]
		Expect(cmp.Diff(*gotTrait, expectScalerTrait("myweb5", app.Name, app.Namespace, gotTrait.GetName(), gotTrait.GetNamespace()))).Should(BeEmpty())

		Expect(len(comp1.Scopes) > 0).Should(BeTrue())
		Expect(*comp1.Scopes[0]).Should(Equal(corev1.ObjectReference{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "HealthScope",
			Name:       "app-with-two-comp-default-health",
		}))
		comp2 := comps[1]
		Expect(len(comp1.Scopes) > 0).Should(BeTrue())
		Expect(*comp2.Scopes[0]).Should(Equal(corev1.ObjectReference{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "HealthScope",
			Name:       "app-with-two-comp-default-health",
		}))

		Expect(comp1.StandardWorkload).ShouldNot(BeNil())
		gotD := &v1.Deployment{}
		Expect(runtime.DefaultUnstructuredConverter.FromUnstructured(comp1.StandardWorkload.Object, gotD)).Should(Succeed())
		gotD.Annotations = nil
		Expect(cmp.Diff(gotD, expDeployment)).Should(BeEmpty())

		expDeployment6 := getExpDeployment("myweb6", app)
		expDeployment6.Spec.Template.Spec.Containers[0].Image = "busybox2"
		Expect(comp2.StandardWorkload).ShouldNot(BeNil())
		gotD2 := &v1.Deployment{}
		Expect(runtime.DefaultUnstructuredConverter.FromUnstructured(comp2.StandardWorkload.Object, gotD2)).Should(Succeed())
		gotD2.Annotations = nil
		Expect(cmp.Diff(gotD2, expDeployment6)).Should(BeEmpty())

		By("Update Application with new revision, component5 with new spec, rename component6 it should create new component ")

		curApp.SetNamespace(app.Namespace)
		curApp.Spec.Components[0] = common.ApplicationComponent{
			Name:       "myweb5",
			Type:       "worker",
			Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox3"}`)},
			Scopes:     map[string]string{"healthscopes.core.oam.dev": "app-with-two-comp-default-health"},
		}
		curApp.Spec.Components[1] = common.ApplicationComponent{
			Name:       "myweb7",
			Type:       "worker",
			Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox"}`)},
			Scopes:     map[string]string{"healthscopes.core.oam.dev": "app-with-two-comp-default-health"},
		}
		Expect(k8sClient.Update(ctx, curApp)).Should(BeNil())
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		By("Check App updated successfully")
		Expect(k8sClient.Get(ctx, appKey, curApp)).Should(BeNil())
		Expect(curApp.Status.ObservedGeneration).Should(Equal(curApp.Generation))
		Expect(curApp.Status.Phase).Should(Equal(common.ApplicationRunning))

		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      curApp.Status.LatestRevision.Name,
		}, appRevision)).Should(BeNil())

		By("Check affiliated resource tracker is upgraded")
		expectRTName = fmt.Sprintf("%s-%s", appRevision.GetName(), appRevision.GetNamespace())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: expectRTName}, &v1beta1.ResourceTracker{})
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		af, err = appParser.GenerateAppFileFromRevision(appRevision)
		Expect(err).Should(BeNil())
		comps, err = af.GenerateComponentManifests()
		Expect(err).Should(BeNil())
		Expect(len(comps) > 0).Should(BeTrue())
		comp1 = comps[0]

		expDeployment.Spec.Template.Spec.Containers[0].Image = "busybox3"
		expDeployment.Labels["app.oam.dev/appRevision"] = app.Name + "-v2"
		Expect(comp1.StandardWorkload).ShouldNot(BeNil())
		gotD = &v1.Deployment{}
		Expect(runtime.DefaultUnstructuredConverter.FromUnstructured(comp1.StandardWorkload.Object, gotD)).Should(Succeed())
		gotD.Annotations = nil
		Expect(cmp.Diff(gotD, expDeployment)).Should(BeEmpty())

		expDeployment7 := getExpDeployment("myweb7", app)
		expDeployment7.Labels["app.oam.dev/appRevision"] = app.Name + "-v2"
		comp2 = comps[1]
		Expect(comp2.StandardWorkload).ShouldNot(BeNil())
		gotD3 := &v1.Deployment{}
		Expect(runtime.DefaultUnstructuredConverter.FromUnstructured(comp2.StandardWorkload.Object, gotD3)).Should(Succeed())
		gotD3.Annotations = nil
		Expect(cmp.Diff(gotD3, expDeployment7)).Should(BeEmpty())

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app-with-trait will create workload and trait with http task", func() {
		s := newMockHTTP()
		defer s.Close()

		By("change trait definition with http task")
		ntd, otd := &v1beta1.TraitDefinition{}, &v1beta1.TraitDefinition{}
		tDDefJson, _ := yaml.YAMLToJSON([]byte(tdDefYamlWithHttp))
		Expect(json.Unmarshal(tDDefJson, ntd)).Should(BeNil())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ntd.Name, Namespace: ntd.Namespace}, otd)).Should(BeNil())
		ntd.ResourceVersion = otd.ResourceVersion
		Expect(k8sClient.Update(ctx, ntd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-trait-http",
			},
		}
		appWithTrait.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		app := appWithTrait.DeepCopy()
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

		af, err := appParser.GenerateAppFileFromRevision(appRevision)
		Expect(err).Should(BeNil())
		comps, err := af.GenerateComponentManifests()
		Expect(err).Should(BeNil())
		Expect(len(comps) > 0).Should(BeTrue())
		comp := comps[0]
		Expect(len(comp.Traits) > 0).Should(BeTrue())
		gotTrait := comp.Traits[0]
		expTrait := expectScalerTrait(appWithTrait.Spec.Components[0].Name, appWithTrait.Name, appWithTrait.Namespace, gotTrait.GetName(), gotTrait.GetNamespace())
		expTrait.Object["spec"].(map[string]interface{})["token"] = "test-token"
		Expect(cmp.Diff(*gotTrait, expTrait)).Should(BeEmpty())

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
	})

	It("app with health policy for workload", func() {
		By("change workload and trait definition with health policy")
		ncd, ocd := &v1beta1.ComponentDefinition{}, &v1beta1.ComponentDefinition{}
		cDDefJson, _ := yaml.YAMLToJSON([]byte(componentDefWithHealthYaml))
		Expect(json.Unmarshal(cDDefJson, ncd)).Should(BeNil())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ncd.Name, Namespace: ncd.Namespace}, ocd)).Should(BeNil())
		ncd.ResourceVersion = ocd.ResourceVersion
		Expect(k8sClient.Update(ctx, ncd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		ntd, otd := &v1beta1.TraitDefinition{}, &v1beta1.TraitDefinition{}
		tDDefJson, _ := yaml.YAMLToJSON([]byte(tDDefWithHealthYaml))
		Expect(json.Unmarshal(tDDefJson, ntd)).Should(BeNil())
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ntd.Name, Namespace: ntd.Namespace}, otd)).Should(BeNil())
		ntd.ResourceVersion = otd.ResourceVersion
		Expect(k8sClient.Update(ctx, ntd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		compName := "myweb-health"
		expDeployment := getExpDeployment(compName, appWithTrait)

		By("create the new namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-with-health",
			},
		}
		appWithTrait.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())

		app := appWithTrait.DeepCopy()
		app.Spec.Components[0].Name = compName
		expDeployment.Name = app.Name
		expDeployment.Namespace = ns.Name
		expDeployment.Labels[oam.LabelAppName] = app.Name
		expDeployment.Labels[oam.LabelAppComponent] = compName
		expDeployment.Labels["app.oam.dev/resourceType"] = "WORKLOAD"
		Expect(k8sClient.Create(ctx, expDeployment)).Should(BeNil())
		expTrait := expectScalerTrait(compName, app.Name, app.Namespace, app.Name, app.Namespace)
		expTrait.SetLabels(map[string]string{
			oam.LabelAppName:         app.Name,
			"trait.oam.dev/type":     "scaler",
			"app.oam.dev/component":  "myweb-health",
			"trait.oam.dev/resource": "scaler",
		})
		(expTrait.Object["spec"].(map[string]interface{}))["workloadRef"] = map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       app.Name,
		}
		Expect(k8sClient.Create(ctx, &expTrait)).Should(BeNil())

		By("enrich the status of deployment and scaler trait")
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())
		got := &v1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, got)).Should(BeNil())
		expTrait.Object["status"] = condition.ConditionedStatus{
			Conditions: []condition.Condition{{
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
			}},
		}
		Expect(k8sClient.Status().Update(ctx, &expTrait)).Should(BeNil())
		tGot := &unstructured.Unstructured{}
		tGot.SetAPIVersion("core.oam.dev/v1alpha2")
		tGot.SetKind("ManualScalerTrait")
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, tGot)).Should(BeNil())

		By("apply appfile")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

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

		By("Check affiliated resource tracker is created")
		expectRTName := fmt.Sprintf("%s-%s", checkApp.Status.LatestRevision.Name, checkApp.GetNamespace())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: expectRTName}, &v1beta1.ResourceTracker{})
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		Expect(k8sClient.Delete(ctx, app)).Should(BeNil())
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
		expDeployment := getExpDeployment(compName, appWithTraitHealthStatus)

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

		expDeployment.Name = app.Name
		expDeployment.Namespace = ns.Name
		expDeployment.Labels[oam.LabelAppName] = app.Name
		expDeployment.Labels[oam.LabelAppComponent] = compName
		expDeployment.Labels["app.oam.dev/resourceType"] = "WORKLOAD"
		Expect(k8sClient.Create(ctx, expDeployment)).Should(BeNil())

		expWorkloadTrait := unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"trait.oam.dev/type":      "AuxiliaryWorkload",
					"app.oam.dev/component":   compName,
					"app.oam.dev/name":        app.Name,
					"trait.oam.dev/resource":  "gameconfig",
					"app.oam.dev/appRevision": app.Name + "-v1",
				},
			},
			"data": map[string]interface{}{
				"enemies": "alien",
				"lives":   "3",
			},
		}}
		expWorkloadTrait.SetName("myweb-health-statusgame-config")
		expWorkloadTrait.SetNamespace(app.Namespace)
		Expect(k8sClient.Create(ctx, &expWorkloadTrait)).Should(BeNil())

		expTrait := unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1beta1",
			"kind":       "Ingress",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"trait.oam.dev/type":      "ingress",
					"trait.oam.dev/resource":  "ingress",
					"app.oam.dev/component":   compName,
					"app.oam.dev/name":        app.Name,
					"app.oam.dev/appRevision": app.Name + "-v1",
				},
			},
			"spec": map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"host": "example.com",
					},
				},
			},
		}}
		expTrait.SetName(compName)
		expTrait.SetNamespace(app.Namespace)
		Expect(k8sClient.Create(ctx, &expTrait)).Should(BeNil())

		expTrait2 := unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					"trait.oam.dev/type":      "ingress",
					"trait.oam.dev/resource":  "service",
					"app.oam.dev/component":   compName,
					"app.oam.dev/name":        app.Name,
					"app.oam.dev/appRevision": app.Name + "-v1",
				},
			},
			"spec": map[string]interface{}{
				"clusterIP": "10.0.0.64",
				"ports": []interface{}{
					map[string]interface{}{
						"port": 80,
					},
				},
			},
		}}
		expTrait2.SetName(app.Name)
		expTrait2.SetNamespace(app.Namespace)
		Expect(k8sClient.Create(ctx, &expTrait2)).Should(BeNil())

		By("enrich the status of deployment and ingress trait")
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())
		got := &v1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: app.Namespace,
			Name:      app.Name,
		}, got)).Should(BeNil())

		By("apply appfile")
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

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
				Message:            "type: busybox,\t enemies:alien",
				Traits: []common.ApplicationTraitStatus{
					{
						Type:    "ingress",
						Healthy: true,
						Message: "type: ClusterIP,\t clusterIP:10.0.0.64,\t ports:80,\t domainexample.com",
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

	PIt("app-import-pkg will create workload by imported kube package", func() {

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-app-import-pkg",
			},
		}
		appImportPkg.SetNamespace(ns.Name)
		expDeployment := getExpDeployment("myweb", appImportPkg)
		expDeployment.Labels["workload.oam.dev/type"] = "worker-import"
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, appImportPkg.DeepCopy())).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      appImportPkg.Name,
			Namespace: appImportPkg.Namespace,
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

		af, err := appParser.GenerateAppFileFromRevision(appRevision)
		Expect(err).Should(BeNil())
		comps, err := af.GenerateComponentManifests()
		Expect(err).Should(BeNil())
		Expect(len(comps) > 0).Should(BeTrue())
		comp := comps[0]

		gotSvc := &corev1.Service{}
		runtime.DefaultUnstructuredConverter.FromUnstructured(comp.Traits[0].Object, gotSvc)
		Expect(cmp.Diff(gotSvc, &corev1.Service{
			TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: "v1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:        "myweb",
				Namespace:   "vela-test-app-import-pkg",
				Annotations: map[string]string{"oam.dev/kubevela-version": "UNKNOWN"},
				Labels: map[string]string{
					"app.oam.dev/component":    "myweb",
					"app.oam.dev/name":         "app-import-pkg",
					"app.oam.dev/namespace":    ns.Name,
					"trait.oam.dev/resource":   "service",
					"trait.oam.dev/type":       "ingress-import",
					"app.oam.dev/appRevision":  "app-import-pkg-v1",
					"app.oam.dev/resourceType": "TRAIT",
				}},
			Spec: corev1.ServiceSpec{
				Ports:    []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt(80)}},
				Selector: map[string]string{"app.oam.dev/component": "myweb"},
			}})).Should(BeEquivalentTo(""))
		gotIngress := &v1beta12.Ingress{}
		runtime.DefaultUnstructuredConverter.FromUnstructured(comp.Traits[1].Object, gotIngress)
		Expect(cmp.Diff(gotIngress, &v1beta12.Ingress{
			TypeMeta: metav1.TypeMeta{Kind: "Ingress", APIVersion: "networking.k8s.io/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{
				Name:        "myweb",
				Namespace:   "vela-test-app-import-pkg",
				Annotations: map[string]string{"oam.dev/kubevela-version": "UNKNOWN"},
				Labels: map[string]string{
					"app.oam.dev/component":    "myweb",
					"app.oam.dev/name":         "app-import-pkg",
					"app.oam.dev/namespace":    ns.Name,
					"trait.oam.dev/resource":   "ingress",
					"app.oam.dev/resourceType": "TRAIT",
					"trait.oam.dev/type":       "ingress-import",
					"app.oam.dev/appRevision":  "app-import-pkg-v1",
				}},
			Spec: v1beta12.IngressSpec{Rules: []v1beta12.IngressRule{{Host: "abc.com",
				IngressRuleValue: v1beta12.IngressRuleValue{HTTP: &v1beta12.HTTPIngressRuleValue{Paths: []v1beta12.HTTPIngressPath{{
					Path:    "/",
					Backend: v1beta12.IngressBackend{ServiceName: "myweb", ServicePort: intstr.FromInt(80)}}}}}}},
			}})).Should(BeEquivalentTo(""))

		By("Check affiliated resource tracker is created")
		expectRTName := fmt.Sprintf("%s-%s", appRevision.GetName(), appRevision.GetNamespace())
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: expectRTName}, &v1beta1.ResourceTracker{})
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		gotD := &v1.Deployment{}
		runtime.DefaultUnstructuredConverter.FromUnstructured(comp.StandardWorkload.Object, gotD)
		gotD.Annotations = nil
		Expect(cmp.Diff(gotD, expDeployment)).Should(BeEmpty())

		By("Delete Application, clean the resource")
		Expect(k8sClient.Delete(ctx, appImportPkg)).Should(BeNil())
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
		appRevName := checkApp.Status.LatestRevision.Name
		checkApp.Spec.Components[0].Traits[0].Properties = &runtime.RawExtension{Raw: []byte(`{"targetRevision":"myweb1-v3"}`)}
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp = &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.LatestRevision.Name).Should(BeEquivalentTo(appRevName))
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
					Steps: []v1beta1.WorkflowStep{
						{
							Name:       "apply",
							Type:       "apply-component",
							Properties: &runtime.RawExtension{Raw: []byte(`{"component" : "myweb1"}`)},
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
		custom.EnableSuspendFailedWorkflow = true
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
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
		Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(workflow.MessageInitializingWorkflow))

		By("verify the first ten reconciles")
		for i := 0; i < custom.MaxWorkflowStepErrorRetryTimes; i++ {
			testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
			Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
			Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
			Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(string(common.WorkflowStateExecuting)))
			Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(common.WorkflowStepPhaseFailed))
		}

		By("application should be suspended after failed max reconciles")
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowSuspending))
		Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(workflow.MessageSuspendFailedAfterRetries))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(common.WorkflowStepPhaseFailed))
		Expect(checkApp.Status.Workflow.Steps[1].Reason).Should(BeEquivalentTo(custom.StatusReasonFailedAfterRetries))

		By("resume the suspended application")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		checkApp.Status.Workflow.Suspend = false
		Expect(k8sClient.Status().Patch(ctx, checkApp, client.Merge)).Should(BeNil())
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
		Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(string(common.WorkflowStateExecuting)))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(common.WorkflowStepPhaseFailed))

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
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))

		for i := 0; i < custom.MaxWorkflowStepErrorRetryTimes-1; i++ {
			testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
			Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
			Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
			Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(string(common.WorkflowStateExecuting)))
			Expect(checkApp.Status.Workflow.Steps[0].Phase).Should(BeEquivalentTo(common.WorkflowStepPhaseRunning))
			Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(common.WorkflowStepPhaseFailed))
		}

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
		Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(string(common.WorkflowStateExecuting)))
		Expect(checkApp.Status.Workflow.Steps[0].Phase).Should(BeEquivalentTo(common.WorkflowStepPhaseRunning))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(common.WorkflowStepPhaseFailed))
		Expect(checkApp.Status.Workflow.Steps[1].Reason).Should(BeEquivalentTo(custom.StatusReasonFailedAfterRetries))
	})

	It("application with step by step workflow failed after retries", func() {
		custom.EnableSuspendFailedWorkflow = true
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
					Steps: []v1beta1.WorkflowStep{
						{
							Name:       "myweb1",
							Type:       "apply-component",
							Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
						},
						{
							Name:       "failed-step",
							Type:       "apply-component",
							Properties: &runtime.RawExtension{Raw: []byte(`{"component":"failed-step"}`)},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
		Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(workflow.MessageInitializingWorkflow))

		By("verify the first twenty reconciles")
		for i := 0; i < custom.MaxWorkflowStepErrorRetryTimes; i++ {
			testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
			Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
			Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
			Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(string(common.WorkflowStateExecuting)))
			Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(common.WorkflowStepPhaseFailed))
		}

		By("application should be suspended after failed max reconciles")
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowSuspending))
		Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(workflow.MessageSuspendFailedAfterRetries))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(common.WorkflowStepPhaseFailed))
		Expect(checkApp.Status.Workflow.Steps[1].Reason).Should(BeEquivalentTo(custom.StatusReasonFailedAfterRetries))

		By("resume the suspended application")
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		checkApp.Status.Workflow.Suspend = false
		Expect(k8sClient.Status().Patch(ctx, checkApp, client.Merge)).Should(BeNil())
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunningWorkflow))
		Expect(checkApp.Status.Workflow.Message).Should(BeEquivalentTo(string(common.WorkflowStateExecuting)))
		Expect(checkApp.Status.Workflow.Steps[1].Phase).Should(BeEquivalentTo(common.WorkflowStepPhaseFailed))
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
					Steps: []v1beta1.WorkflowStep{
						{
							Name:       "myweb1",
							Type:       "apply-component",
							Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
						},
						{
							Name: "myweb2",
							Type: "step-group",
							SubSteps: []common.WorkflowSubStep{
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
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp = &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
	})

	It("application with if always in workflow", func() {
		custom.EnableSuspendFailedWorkflow = false
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
					Steps: []v1beta1.WorkflowStep{
						{
							Name:       "failed-step",
							Type:       "apply-component",
							Properties: &runtime.RawExtension{Raw: []byte(`{"component":"failed-step"}`)},
						},
						{
							Name:       "myweb1",
							Type:       "apply-component",
							If:         "always",
							Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb1"}`)},
						},
						{
							Name:       "myweb2",
							Type:       "apply-component",
							Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		By("verify the first ten reconciles")
		for i := 0; i < custom.MaxWorkflowStepErrorRetryTimes; i++ {
			testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		}

		expDeployment := &v1.Deployment{}
		web1Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb1"}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(util.NotFoundMatcher{})
		web2Key := types.NamespacedName{Namespace: ns.Name, Name: "myweb2"}
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowTerminated))
	})

	It("application with if always in workflow sub steps", func() {
		custom.EnableSuspendFailedWorkflow = false
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
					Steps: []v1beta1.WorkflowStep{
						{
							Name: "myweb1",
							Type: "step-group",
							SubSteps: []common.WorkflowSubStep{
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
							Name:       "myweb2",
							Type:       "apply-component",
							If:         "always",
							Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb2"}`)},
						},
						{
							Name:       "myweb3",
							Type:       "apply-component",
							Properties: &runtime.RawExtension{Raw: []byte(`{"component":"myweb3"}`)},
						},
					},
				},
			},
		}

		Expect(k8sClient.Create(context.Background(), app)).Should(BeNil())
		appKey := types.NamespacedName{Namespace: ns.Name, Name: app.Name}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		By("verify the first ten reconciles")
		for i := 0; i < custom.MaxWorkflowStepErrorRetryTimes; i++ {
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
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(util.NotFoundMatcher{})
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})

		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		Expect(k8sClient.Get(ctx, web2Key, expDeployment)).Should(BeNil())
		Expect(k8sClient.Get(ctx, web3Key, expDeployment)).Should(util.NotFoundMatcher{})
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationWorkflowTerminated))
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
						Properties: &runtime.RawExtension{Raw: []byte(`{"cmd":["sleep"],"image":"busybox"}`)},
						Inputs: common.StepInputs{
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
						Outputs: common.StepOutputs{
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
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		expDeployment = &v1.Deployment{}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())
		Expect(expDeployment.Spec.Template.Spec.Containers[0].Command).Should(BeEquivalentTo([]string{"sleep", "100"}))
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
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
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment = &v1.Deployment{}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
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
						Inputs: common.StepInputs{
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
						Outputs: common.StepOutputs{
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
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})

		expDeployment = &v1.Deployment{}
		Expect(k8sClient.Get(ctx, web1Key, expDeployment)).Should(BeNil())
		expDeployment.Status.Replicas = 1
		expDeployment.Status.ReadyReplicas = 1
		Expect(k8sClient.Status().Update(ctx, expDeployment)).Should(BeNil())

		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
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

	It("app record execution state with controllerRevision", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-app-trace",
			},
		}

		app := appwithNoTrait.DeepCopy()
		app.Name = "vela-test-app-trace"
		app.SetNamespace(ns.Name)
		app.Annotations = map[string]string{oam.AnnotationPublishVersion: "v134"}
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      app.Name,
			Namespace: app.Namespace,
		}

		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp := &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
		recorder := &v1.ControllerRevision{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      fmt.Sprintf("record-%s-v134", app.Name),
			Namespace: app.Namespace,
		}, recorder)).Should(BeNil())

		web := &v1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      "myweb2",
			Namespace: app.Namespace,
		}, web)).Should(BeNil())
		web.Spec.Replicas = pointer.Int32(0)
		Expect(k8sClient.Update(ctx, web)).Should(BeNil())

		checkApp.Annotations[oam.AnnotationPublishVersion] = "v135"
		Expect(k8sClient.Update(ctx, checkApp)).Should(BeNil())
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})
		checkApp = &v1beta1.Application{}
		Expect(k8sClient.Get(ctx, appKey, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Phase).Should(BeEquivalentTo(common.ApplicationRunning))
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      fmt.Sprintf("record-%s-v135", app.Name),
			Namespace: app.Namespace,
		}, recorder)).Should(BeNil())

		checkWeb := &v1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      "myweb2",
			Namespace: app.Namespace,
		}, checkWeb)).Should(BeNil())
		Expect(*(checkWeb.Spec.Replicas)).Should(BeEquivalentTo(int32(0)))
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
			Steps: []v1beta1.WorkflowStep{{
				Name:       "apply-in-parallel",
				Type:       "apply-test",
				Properties: &runtime.RawExtension{Raw: []byte(`{"parallelism": 20}`)},
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
		ingress := &v1beta12.Ingress{}
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
				},
				Policies: []v1beta1.AppPolicy{
					{
						Type: "debug",
						Name: "debug",
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
			Name:      debug.GenerateContextName(app.Name, "myworker"),
			Namespace: "default",
		}, debugCM)).Should(BeNil())

		By("Update the application to update the debug Config Map")
		app.Spec.Components[0].Properties = &runtime.RawExtension{Raw: []byte("{\"cmd\":[\"sleep\",\"1000\"],\"image\":\"busybox\",\"env\":[{\"name\":\"firstKey\",\"value\":\"updateValue\"}]}")}
		testutil.ReconcileOnce(reconciler, reconcile.Request{NamespacedName: appKey})
		updatedCM := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      debug.GenerateContextName(app.Name, "myworker"),
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

		Expect(k8sClient.Delete(ctx, cm)).Should(BeNil())
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
        	network "k8s.io/networking/v1beta1"
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
	gatewayYaml = `apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: Enable public web traffic for the component, the ingress API matches K8s v1.20+.
  name: gateway
  namespace: vela-system
spec:
  appliesToWorkloads:
    - '*'
  podDisruptive: false
  schematic:
    cue:
      template: |
        // trait template can have multiple outputs in one trait
        outputs: service: {
        	apiVersion: "v1"
        	kind:       "Service"
        	metadata: name: context.name
        	spec: {
        		selector: "app.oam.dev/component": context.name
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
        	metadata: {
        		name: context.name
        		annotations: {
        			if !parameter.classInSpec {
        				"kubernetes.io/ingress.class": parameter.class
        			}
        		}
        	}
        	spec: {
        		if parameter.classInSpec {
        			ingressClassName: parameter.class
        		}
        		if parameter.secretName != _|_ {
        			tls: [{
        				hosts: [
        					parameter.domain,
        				]
        				secretName: parameter.secretName
        			}]
        		}
        		rules: [{
        			host: parameter.domain
        			http: paths: [
        				for k, v in parameter.http {
        					path:     k
        					pathType: "ImplementationSpecific"
        					backend: service: {
        						name: context.name
        						port: number: v
        					}
        				},
        			]
        		}]
        	}
        }
        parameter: {
        	// +usage=Specify the domain you want to expose
        	domain: string

        	// +usage=Specify the mapping relationship between the http path and the workload port
        	http: [string]: int

        	// +usage=Specify the class of ingress to use
        	class: *"nginx" | string

        	// +usage=Set ingress class in '.spec.ingressClassName' instead of 'kubernetes.io/ingress.class' annotation.
        	classInSpec: *false | bool

        	// +usage=Specify the secret name you want to quote.
        	secretName?: string
        }
  status:
    customStatus: |-
      let igs = context.outputs.ingress.status.loadBalancer.ingress
      if igs == _|_ {
        message: "No loadBalancer found, visiting by using 'vela port-forward " + context.appName + "'\n"
      }
      if len(igs) > 0 {
        if igs[0].ip != _|_ {
      	  message: "Visiting URL: " + context.outputs.ingress.spec.rules[0].host + ", IP: " + igs[0].ip
        }
        if igs[0].ip == _|_ {
      	  message: "Visiting URL: " + context.outputs.ingress.spec.rules[0].host
        }
      }
    healthPolicy: 'isHealth: len(context.outputs.service.spec.clusterIP) > 0'

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
	traitDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Manually scale the app"
  name: scaler
  namespace: vela-system
spec:
  appliesToWorkloads:
    - deployments.apps
  definitionRef:
    name: manualscalertraits.core.oam.dev
  workloadRefPath: spec.workloadRef
  extension:
    template: |-
      outputs: scaler: {
      	apiVersion: "core.oam.dev/v1alpha2"
      	kind:       "ManualScalerTrait"
      	spec: {
      		replicaCount: parameter.replicas
      	}
      }
      parameter: {
      	//+short=r
      	replicas: *1 | int
      }

`
	tdDefYamlWithHttp = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Manually scale the app"
  name: scaler
  namespace: vela-system
spec:
  appliesToWorkloads:
    - deployments.apps
  definitionRef:
    name: manualscalertraits.core.oam.dev
  workloadRefPath: spec.workloadRef
  extension:
    template: |-
      outputs: scaler: {
      	apiVersion: "core.oam.dev/v1alpha2"
      	kind:       "ManualScalerTrait"
      	spec: {
          replicaCount: parameter.replicas
          token: processing.output.token
      	}
      }
      parameter: {
      	//+short=r
        replicas: *1 | int
        serviceURL: *"http://127.0.0.1:8090/api/v1/token?val=test-token" | string
      }
      processing: {
        output: {
          token ?: string
        }
        http: {
          method: *"GET" | string
          url: parameter.serviceURL
          request: {
              body ?: bytes
              header: {}
              trailer: {}
          }
        }
      }
`
	tDDefWithHealthYaml = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Manually scale the app"
  name: scaler
  namespace: vela-system
spec:
  appliesToWorkloads:
    - deployments.apps
  definitionRef:
    name: manualscalertraits.core.oam.dev
  workloadRefPath: spec.workloadRef
  extension:
    healthPolicy: |
      isHealth: context.output.status.conditions[0].status == "True"
    template: |-
      outputs: scaler: {
      	apiVersion: "core.oam.dev/v1alpha2"
      	kind:       "ManualScalerTrait"
      	spec: {
      		replicaCount: parameter.replicas
      	}
      }
      parameter: {
      	//+short=r
      	replicas: *1 | int
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

	storageYaml = `apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: Add storages on K8s pod for your workload which follows the pod spec in path 'spec.template'.
  name: storage
  namespace: vela-system
spec:
  appliesToWorkloads:
    - deployments.apps
  podDisruptive: true
  schematic:
    cue:
      template: |
        pvcVolumesList: *[
        		for v in parameter.pvc {
        		{
        			name: "pvc-" + v.name
        			persistentVolumeClaim: claimName: v.name
        		}
        	},
        ] | []
        configMapVolumesList: *[
        			for v in parameter.configMap if v.mountPath != _|_ {
        		{
        			name: "configmap-" + v.name
        			configMap: {
        				defaultMode: v.defaultMode
        				name:        v.name
        				if v.items != _|_ {
        					items: v.items
        				}
        			}
        		}
        	},
        ] | []
        secretVolumesList: *[
        			for v in parameter.secret if v.mountPath != _|_ {
        		{
        			name: "secret-" + v.name
        			secret: {
        				defaultMode: v.defaultMode
        				secretName:  v.name
        				if v.items != _|_ {
        					items: v.items
        				}
        			}
        		}
        	},
        ] | []
        emptyDirVolumesList: *[
        			for v in parameter.emptyDir {
        		{
        			name: "emptydir-" + v.name
        			emptyDir: medium: v.medium
        		}
        	},
        ] | []
        pvcVolumeMountsList: *[
        			for v in parameter.pvc {
        		if v.volumeMode == "Filesystem" {
        			{
        				name:      "pvc-" + v.name
        				mountPath: v.mountPath
        			}
        		}
        	},
        ] | []
        configMapVolumeMountsList: *[
        				for v in parameter.configMap if v.mountPath != _|_ {
        		{
        			name:      "configmap-" + v.name
        			mountPath: v.mountPath
        		}
        	},
        ] | []
        configMapEnvMountsList: *[
        			for v in parameter.configMap if v.mountToEnv != _|_ {
        		{
        			name: v.mountToEnv.envName
        			valueFrom: configMapKeyRef: {
        				name: v.name
        				key:  v.mountToEnv.configMapKey
        			}
        		}
        	},
        ] | []
        configMapMountToEnvsList: *[
        			for v in parameter.configMap if v.mountToEnvs != _|_ for k in v.mountToEnvs {
        		{
        			name: k.envName
        			valueFrom: configMapKeyRef: {
        				name: v.name
        				key:  k.configMapKey
        			}
        		}
        	},
        ] | []
        secretVolumeMountsList: *[
        			for v in parameter.secret if v.mountPath != _|_ {
        		{
        			name:      "secret-" + v.name
        			mountPath: v.mountPath
        		}
        	},
        ] | []
        secretEnvMountsList: *[
        			for v in parameter.secret if v.mountToEnv != _|_ {
        		{
        			name: v.mountToEnv.envName
        			valueFrom: secretKeyRef: {
        				name: v.name
        				key:  v.mountToEnv.secretKey
        			}
        		}
        	},
        ] | []
        secretMountToEnvsList: *[
        			for v in parameter.secret if v.mountToEnvs != _|_ for k in v.mountToEnvs {
        		{
        			name: k.envName
        			valueFrom: secretKeyRef: {
        				name: v.name
        				key:  k.secretKey
        			}
        		}
        	},
        ] | []
        emptyDirVolumeMountsList: *[
        				for v in parameter.emptyDir {
        		{
        			name:      "emptydir-" + v.name
        			mountPath: v.mountPath
        		}
        	},
        ] | []
        volumeDevicesList: *[
        			for v in parameter.pvc if v.volumeMode == "Block" {
        		{
        			name:       "pvc-" + v.name
        			devicePath: v.mountPath
        		}
        	},
        ] | []
        patch: spec: template: spec: {
        	// +patchKey=name
        	volumes: pvcVolumesList + configMapVolumesList + secretVolumesList + emptyDirVolumesList

        	containers: [{
        		// +patchKey=name
        		env: configMapEnvMountsList + secretEnvMountsList + configMapMountToEnvsList + secretMountToEnvsList
        		// +patchKey=name
        		volumeDevices: volumeDevicesList
        		// +patchKey=name
        		volumeMounts: pvcVolumeMountsList + configMapVolumeMountsList + secretVolumeMountsList + emptyDirVolumeMountsList
        	},...]

        }
        outputs: {
        	for v in parameter.pvc {
        		if v.mountOnly == false {
        			"pvc-\(v.name)": {
        				apiVersion: "v1"
        				kind:       "PersistentVolumeClaim"
        				metadata: name: v.name
        				spec: {
        					accessModes: v.accessModes
        					volumeMode:  v.volumeMode
        					if v.volumeName != _|_ {
        						volumeName: v.volumeName
        					}
        					if v.storageClassName != _|_ {
        						storageClassName: v.storageClassName
        					}

        					if v.resources.requests.storage == _|_ {
        						resources: requests: storage: "8Gi"
        					}
        					if v.resources.requests.storage != _|_ {
        						resources: requests: storage: v.resources.requests.storage
        					}
        					if v.resources.limits.storage != _|_ {
        						resources: limits: storage: v.resources.limits.storage
        					}
        					if v.dataSourceRef != _|_ {
        						dataSourceRef: v.dataSourceRef
        					}
        					if v.dataSource != _|_ {
        						dataSource: v.dataSource
        					}
        					if v.selector != _|_ {
        						dataSource: v.selector
        					}
        				}
        			}
        		}
        	}

        	for v in parameter.configMap {
        		if v.mountOnly == false {
        			"configmap-\(v.name)": {
        				apiVersion: "v1"
        				kind:       "ConfigMap"
        				metadata: name: v.name
        				if v.data != _|_ {
        					data: v.data
        				}
        			}
        		}
        	}

        	for v in parameter.secret {
        		if v.mountOnly == false {
        			"secret-\(v.name)": {
        				apiVersion: "v1"
        				kind:       "Secret"
        				metadata: name: v.name
        				if v.data != _|_ {
        					data: v.data
        				}
        				if v.stringData != _|_ {
        					stringData: v.stringData
        				}
        			}
        		}
        	}

        }
        parameter: {
        	// +usage=Declare pvc type storage
        	pvc?: [...{
        		name:              string
        		mountOnly:         *false | bool
        		mountPath:         string
        		volumeMode:        *"Filesystem" | string
        		volumeName?:       string
        		accessModes:       *["ReadWriteOnce"] | [...string]
        		storageClassName?: string
        		resources?: {
        			requests: storage: =~"^([1-9][0-9]{0,63})(E|P|T|G|M|K|Ei|Pi|Ti|Gi|Mi|Ki)$"
        			limits?: storage:  =~"^([1-9][0-9]{0,63})(E|P|T|G|M|K|Ei|Pi|Ti|Gi|Mi|Ki)$"
        		}
        		dataSourceRef?: {
        			name:     string
        			kind:     string
        			apiGroup: string
        		}
        		dataSource?: {
        			name:     string
        			kind:     string
        			apiGroup: string
        		}
        		selector?: {
        			matchLabels?: [string]: string
        			matchExpressions?: {
        				key: string
        				values: [...string]
        				operator: string
        			}
        		}
        	}]

        	// +usage=Declare config map type storage
        	configMap?: [...{
        		name:      string
        		mountOnly: *false | bool
        		mountToEnv?: {
        			envName:      string
        			configMapKey: string
        		}
        		mountToEnvs?: [...{
        			envName:      string
        			configMapKey: string
        		}]
        		mountPath?:   string
        		defaultMode: *420 | int
        		readOnly:    *false | bool
        		data?: {...}
        		items?: [...{
        			key:  string
        			path: string
        			mode: *511 | int
        		}]
        	}]

        	// +usage=Declare secret type storage
        	secret?: [...{
        		name:      string
        		mountOnly: *false | bool
        		mountToEnv?: {
        			envName:   string
        			secretKey: string
        		}
        		mountToEnvs?: [...{
        			envName:   string
        			secretKey: string
        		}]
        		mountPath?:   string
        		defaultMode: *420 | int
        		readOnly:    *false | bool
        		stringData?: {...}
        		data?: {...}
        		items?: [...{
        			key:  string
        			path: string
        			mode: *511 | int
        		}]
        	}]

        	// +usage=Declare empty dir type storage
        	emptyDir?: [...{
        		name:      string
        		mountPath: string
        		medium:    *"" | "Memory"
        	}]
        }
`

	envYaml = `apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: Add env on K8s pod for your workload which follows the pod spec in path 'spec.template'
  labels:
    custom.definition.oam.dev/ui-hidden: "true"
  name: env
  namespace: vela-system
spec:
  appliesToWorkloads:
    - '*'
  schematic:
    cue:
      template: |
        #PatchParams: {
        	// +usage=Specify the name of the target container, if not set, use the component name
        	containerName: *"" | string
        	// +usage=Specify if replacing the whole environment settings for the container
        	replace: *false | bool
        	// +usage=Specify the  environment variables to merge, if key already existing, override its value
        	env: [string]: string
        	// +usage=Specify which existing environment variables to unset
        	unset: *[] | [...string]
        }
        PatchContainer: {
        	_params: #PatchParams
        	name:    _params.containerName
        	_delKeys: {for k in _params.unset {"\(k)": ""}}
        	_baseContainers: context.output.spec.template.spec.containers
        	_matchContainers_: [ for _container_ in _baseContainers if _container_.name == name {_container_}]
        	_baseContainer: *_|_ | {...}
        	if len(_matchContainers_) == 0 {
        		err: "container \(name) not found"
        	}
        	if len(_matchContainers_) > 0 {
        		_baseContainer: _matchContainers_[0]
        		_baseEnv:       _baseContainer.env
        		if _baseEnv == _|_ {
        			// +patchStrategy=replace
        			env: [ for k, v in _params.env if _delKeys[k] == _|_ {
        				name:  k
        				value: v
        			}]
        		}
        		if _baseEnv != _|_ {
        			_baseEnvMap: {for envVar in _baseEnv {"\(envVar.name)": envVar.value}}
        			// +patchStrategy=replace
        			env: [ for envVar in _baseEnv if _delKeys[envVar.name] == _|_ && !_params.replace {
        				name: envVar.name
        				if _params.env[envVar.name] != _|_ {
        					value: _params.env[envVar.name]
        				}
        				if _params.env[envVar.name] == _|_ {
        					value: envVar.value
        				}
        			}] + [ for k, v in _params.env if _delKeys[k] == _|_ && (_params.replace || _baseEnvMap[k] == _|_) {
        				name:  k
        				value: v
        			}]
        		}
        	}
        }
        patch: spec: template: spec: {
        	if parameter.containers == _|_ {
        		// +patchKey=name
        		containers: [{
        			PatchContainer & {_params: {
        				if parameter.containerName == "" {
        					containerName: context.name
        				}
        				if parameter.containerName != "" {
        					containerName: parameter.containerName
        				}
        				replace: parameter.replace
        				env:     parameter.env
        				unset:   parameter.unset
        			}}
        		}]
        	}
        	if parameter.containers != _|_ {
        		// +patchKey=name
        		containers: [ for c in parameter.containers {
        			if c.containerName == "" {
        				err: "containerName must be set for containers"
        			}
        			if c.containerName != "" {
        				PatchContainer & {_params: c}
        			}
        		}]
        	}
        }
        parameter: *#PatchParams | close({
        	// +usage=Specify the environment variables for multiple containers
        	containers: [...#PatchParams]
        })
        errs: [ for c in patch.spec.template.spec.containers if c.err != _|_ {c.err}]

`

	hubCpuScalerYaml = `apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: Automatically scale the component based on CPU usage.
  labels:
    custom.definition.oam.dev/ui-hidden: "true"
  name: hubcpuscaler
  namespace: vela-system
spec:
  appliesToWorkloads:
    - deployments.apps
  controlPlaneOnly: true
  schematic:
    cue:
      template: |
        outputs: hubcpuscaler: {
        	apiVersion: "autoscaling/v1"
        	kind:       "HorizontalPodAutoscaler"
        	metadata: name: context.name
        	spec: {
        		scaleTargetRef: {
        			apiVersion: parameter.targetAPIVersion
        			kind:       parameter.targetKind
        			name:       context.name
        		}
        		minReplicas:                    parameter.min
        		maxReplicas:                    parameter.max
        		targetCPUUtilizationPercentage: parameter.cpuUtil
        	}
        }
        parameter: {
        	// +usage=Specify the minimal number of replicas to which the autoscaler can scale down
        	min: *1 | int
        	// +usage=Specify the maximum number of of replicas to which the autoscaler can scale up
        	max: *10 | int
        	// +usage=Specify the average CPU utilization, for example, 50 means the CPU usage is 50%
        	cpuUtil: *50 | int
        	// +usage=Specify the apiVersion of scale target
        	targetAPIVersion: *"apps/v1" | string
        	// +usage=Specify the kind of scale target
        	targetKind: *"Deployment" | string
        }
`
	affinityYaml = `apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: affinity specify affinity and tolerationon K8s pod for your workload which follows the pod spec in path 'spec.template'.
  labels:
    custom.definition.oam.dev/ui-hidden: "true"
  name: affinity
  namespace: vela-system
spec:
  appliesToWorkloads:
    - '*'
  podDisruptive: true
  schematic:
    cue:
      template: |
        patch: spec: template: spec: {
        	if parameter.podAffinity != _|_ {
        		affinity: podAffinity: {
        			if parameter.podAffinity.required != _|_ {
        				requiredDuringSchedulingIgnoredDuringExecution: [
        					for k in parameter.podAffinity.required {
        						if k.labelSelector != _|_ {
        							labelSelector: k.labelSelector
        						}
        						if k.namespace != _|_ {
        							namespace: k.namespace
        						}
        						topologyKey: k.topologyKey
        						if k.namespaceSelector != _|_ {
        							namespaceSelector: k.namespaceSelector
        						}
        					}]
        			}
        			if parameter.podAffinity.preferred != _|_ {
        				preferredDuringSchedulingIgnoredDuringExecution: [
        					for k in parameter.podAffinity.preferred {
        						weight:          k.weight
        						podAffinityTerm: k.podAffinityTerm
        					}]
        			}
        		}
        	}
        	if parameter.podAntiAffinity != _|_ {
        		affinity: podAntiAffinity: {
        			if parameter.podAntiAffinity.required != _|_ {
        				requiredDuringSchedulingIgnoredDuringExecution: [
        					for k in parameter.podAntiAffinity.required {
        						if k.labelSelector != _|_ {
        							labelSelector: k.labelSelector
        						}
        						if k.namespace != _|_ {
        							namespace: k.namespace
        						}
        						topologyKey: k.topologyKey
        						if k.namespaceSelector != _|_ {
        							namespaceSelector: k.namespaceSelector
        						}
        					}]
        			}
        			if parameter.podAntiAffinity.preferred != _|_ {
        				preferredDuringSchedulingIgnoredDuringExecution: [
        					for k in parameter.podAntiAffinity.preferred {
        						weight:          k.weight
        						podAffinityTerm: k.podAffinityTerm
        					}]
        			}
        		}
        	}
        	if parameter.nodeAffinity != _|_ {
        		affinity: nodeAffinity: {
        			if parameter.nodeAffinity.required != _|_ {
        				requiredDuringSchedulingIgnoredDuringExecution: nodeSelectorTerms: [
        					for k in parameter.nodeAffinity.required.nodeSelectorTerms {
        						if k.matchExpressions != _|_ {
        							matchExpressions: k.matchExpressions
        						}
        						if k.matchFields != _|_ {
        							matchFields: k.matchFields
        						}
        					}]
        			}
        			if parameter.nodeAffinity.preferred != _|_ {
        				preferredDuringSchedulingIgnoredDuringExecution: [
        					for k in parameter.nodeAffinity.preferred {
        						weight:     k.weight
        						preference: k.preference
        					}]
        			}
        		}
        	}
        	if parameter.tolerations != _|_ {
        		tolerations: [
        			for k in parameter.tolerations {
        				if k.key != _|_ {
        					key: k.key
        				}
        				if k.effect != _|_ {
        					effect: k.effect
        				}
        				if k.value != _|_ {
        					value: k.value
        				}
        				operator: k.operator
        				if k.tolerationSeconds != _|_ {
        					tolerationSeconds: k.tolerationSeconds
        				}
        			}]
        	}
        }
        #labelSelector: {
        	matchLabels?: [string]: string
        	matchExpressions?: [...{
        		key:      string
        		operator: *"In" | "NotIn" | "Exists" | "DoesNotExist"
        		values?: [...string]
        	}]
        }
        #podAffinityTerm: {
        	labelSelector?: #labelSelector
        	namespaces?: [...string]
        	topologyKey:        string
        	namespaceSelector?: #labelSelector
        }
        #nodeSelecor: {
        	key:      string
        	operator: *"In" | "NotIn" | "Exists" | "DoesNotExist" | "Gt" | "Lt"
        	values?: [...string]
        }
        #nodeSelectorTerm: {
        	matchExpressions?: [...#nodeSelecor]
        	matchFields?: [...#nodeSelecor]
        }
        parameter: {
        	// +usage=Specify the pod affinity scheduling rules
        	podAffinity?: {
        		// +usage=Specify the required during scheduling ignored during execution
        		required?: [...#podAffinityTerm]
        		// +usage=Specify the preferred during scheduling ignored during execution
        		preferred?: [...{
        			// +usage=Specify weight associated with matching the corresponding podAffinityTerm
        			weight: int & >=1 & <=100
        			// +usage=Specify a set of pods
        			podAffinityTerm: #podAffinityTerm
        		}]
        	}
        	// +usage=Specify the pod anti-affinity scheduling rules
        	podAntiAffinity?: {
        		// +usage=Specify the required during scheduling ignored during execution
        		required?: [...#podAffinityTerm]
        		// +usage=Specify the preferred during scheduling ignored during execution
        		preferred?: [...{
        			// +usage=Specify weight associated with matching the corresponding podAffinityTerm
        			weight: int & >=1 & <=100
        			// +usage=Specify a set of pods
        			podAffinityTerm: #podAffinityTerm
        		}]
        	}
        	// +usage=Specify the node affinity scheduling rules for the pod
        	nodeAffinity?: {
        		// +usage=Specify the required during scheduling ignored during execution
        		required?: {
        			// +usage=Specify a list of node selector
        			nodeSelectorTerms: [...#nodeSelectorTerm]
        		}
        		// +usage=Specify the preferred during scheduling ignored during execution
        		preferred?: [...{
        			// +usage=Specify weight associated with matching the corresponding nodeSelector
        			weight: int & >=1 & <=100
        			// +usage=Specify a node selector
        			preference: #nodeSelectorTerm
        		}]
        	}
        	// +usage=Specify tolerant taint
        	tolerations?: [...{
        		key?:     string
        		operator: *"Equal" | "Exists"
        		value?:   string
        		effect?:  "NoSchedule" | "PreferNoSchedule" | "NoExecute"
        		// +usage=Specify the period of time the toleration
        		tolerationSeconds?: int
        	}]
        }
`
)

func newMockHTTP() *httptest.Server {
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			fmt.Printf("Expected 'GET' request, got '%s'", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/token" {
			fmt.Printf("Expected request to '/person', got '%s'", r.URL.EscapedPath())
		}
		r.ParseForm()
		token := r.Form.Get("val")
		tokenBytes, _ := json.Marshal(map[string]interface{}{"token": token})

		w.WriteHeader(http.StatusOK)
		w.Write(tokenBytes)
	}))
	l, _ := net.Listen("tcp", "127.0.0.1:8090")
	ts.Listener.Close()
	ts.Listener = l
	ts.Start()
	return ts
}
