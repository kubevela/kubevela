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

package component

import (
	"context"
	"math/rand"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/rest"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/features"
	pkgcommon "github.com/oam-dev/kubevela/pkg/utils/common"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Utils Suite")
}

var _ = BeforeSuite(func(done Done) {
	rand.Seed(time.Now().UnixNano())
	By("bootstrapping test environment for utils test")

	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.Bool(false),
		CRDDirectoryPaths:        []string{"./testdata"},
	}

	By("start kube test env")
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	By("new kube client")
	cfg.Timeout = time.Minute * 2
	k8sClient, err = client.New(cfg, client.Options{Scheme: pkgcommon.Scheme})
	Expect(err).Should(Succeed())
	close(done)
}, 240)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Test ref-objects functions", func() {
	It("Test SelectRefObjectsForDispatch", func() {
		defer featuregatetesting.SetFeatureGateDuringTest(&testing.T{}, utilfeature.DefaultFeatureGate, features.LegacyObjectTypeIdentifier, true)()
		defer featuregatetesting.SetFeatureGateDuringTest(&testing.T{}, utilfeature.DefaultFeatureGate, features.DeprecatedObjectLabelSelector, true)()
		By("Create objects")
		Expect(k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}})).Should(Succeed())
		for _, obj := range []client.Object{&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dynamic",
				Namespace: "test",
			},
		}, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "dynamic",
				Namespace:  "test",
				Generation: int64(5),
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.0.0.254",
				Ports:     []corev1.ServicePort{{Port: 80}},
			},
		}, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "by-label-1",
				Namespace: "test",
				Labels:    map[string]string{"key": "value"},
			},
		}, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "by-label-2",
				Namespace: "test",
				Labels:    map[string]string{"key": "value"},
			},
		}, &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cluster-role",
			},
		}} {
			Expect(k8sClient.Create(context.Background(), obj)).Should(Succeed())
		}
		createUnstructured := func(apiVersion string, kind string, name string, namespace string, labels map[string]interface{}) *unstructured.Unstructured {
			un := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": apiVersion,
					"kind":       kind,
					"metadata":   map[string]interface{}{"name": name},
				},
			}
			if namespace != "" {
				un.SetNamespace(namespace)
			}
			if labels != nil {
				un.Object["metadata"].(map[string]interface{})["labels"] = labels
			}
			return un
		}
		testcases := map[string]struct {
			Input         v1alpha1.ObjectReferrer
			compName      string
			appNs         string
			Output        []*unstructured.Unstructured
			Error         string
			Scope         string
			IsService     bool
			IsClusterRole bool
		}{
			"normal": {
				Input: v1alpha1.ObjectReferrer{
					ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
					ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic"},
				},
				appNs:  "test",
				Output: []*unstructured.Unstructured{createUnstructured("v1", "ConfigMap", "dynamic", "test", nil)},
			},
			"legacy-type-identifier": {
				Input: v1alpha1.ObjectReferrer{
					ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{LegacyObjectTypeIdentifier: v1alpha1.LegacyObjectTypeIdentifier{Kind: "ConfigMap", APIVersion: "v1"}},
					ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic"},
				},
				appNs:  "test",
				Output: []*unstructured.Unstructured{createUnstructured("v1", "ConfigMap", "dynamic", "test", nil)},
			},
			"invalid-apiVersion": {
				Input: v1alpha1.ObjectReferrer{
					ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{LegacyObjectTypeIdentifier: v1alpha1.LegacyObjectTypeIdentifier{Kind: "ConfigMap", APIVersion: "a/b/v1"}},
					ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic"},
				},
				appNs: "test",
				Error: "invalid APIVersion",
			},
			"invalid-type-identifier": {
				Input: v1alpha1.ObjectReferrer{
					ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{},
					ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic"},
				},
				appNs: "test",
				Error: "neither resource or apiVersion/kind is set",
			},
			"name-and-selector-both-set": {
				Input: v1alpha1.ObjectReferrer{ObjectSelector: v1alpha1.ObjectSelector{Name: "dynamic", LabelSelector: map[string]string{"key": "value"}}},
				appNs: "test",
				Error: "invalid object selector for ref-objects, name and labelSelector cannot be both set",
			},
			"empty-ref-object-name": {
				Input:    v1alpha1.ObjectReferrer{ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"}},
				compName: "dynamic",
				appNs:    "test",
				Output:   []*unstructured.Unstructured{createUnstructured("v1", "ConfigMap", "dynamic", "test", nil)},
			},
			"cannot-find-ref-object": {
				Input: v1alpha1.ObjectReferrer{
					ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
					ObjectSelector:       v1alpha1.ObjectSelector{Name: "static"},
				},
				appNs: "test",
				Error: "failed to load ref object",
			},
			"modify-service": {
				Input: v1alpha1.ObjectReferrer{
					ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "service"},
					ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic"},
				},
				appNs:     "test",
				IsService: true,
			},
			"by-labels": {
				Input: v1alpha1.ObjectReferrer{
					ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
					ObjectSelector:       v1alpha1.ObjectSelector{LabelSelector: map[string]string{"key": "value"}},
				},
				appNs: "test",
				Output: []*unstructured.Unstructured{
					createUnstructured("v1", "ConfigMap", "by-label-1", "test", map[string]interface{}{"key": "value"}),
					createUnstructured("v1", "ConfigMap", "by-label-2", "test", map[string]interface{}{"key": "value"}),
				},
			},
			"by-deprecated-labels": {
				Input: v1alpha1.ObjectReferrer{
					ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
					ObjectSelector:       v1alpha1.ObjectSelector{DeprecatedLabelSelector: map[string]string{"key": "value"}},
				},
				appNs: "test",
				Output: []*unstructured.Unstructured{
					createUnstructured("v1", "ConfigMap", "by-label-1", "test", map[string]interface{}{"key": "value"}),
					createUnstructured("v1", "ConfigMap", "by-label-2", "test", map[string]interface{}{"key": "value"}),
				},
			},
			"no-kind-for-resource": {
				Input: v1alpha1.ObjectReferrer{ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "unknown"}},
				appNs: "test",
				Error: "no matches",
			},
			"cross-namespace": {
				Input: v1alpha1.ObjectReferrer{
					ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
					ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic", Namespace: "test"},
				},
				appNs:  "demo",
				Output: []*unstructured.Unstructured{createUnstructured("v1", "ConfigMap", "dynamic", "test", nil)},
			},
			"cross-namespace-forbidden": {
				Input: v1alpha1.ObjectReferrer{
					ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
					ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic", Namespace: "test"},
				},
				appNs: "demo",
				Scope: RefObjectsAvailableScopeNamespace,
				Error: "cannot refer to objects outside the application's namespace",
			},
			"cross-cluster": {
				Input: v1alpha1.ObjectReferrer{
					ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
					ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic", Cluster: "demo"},
				},
				appNs:  "test",
				Output: []*unstructured.Unstructured{createUnstructured("v1", "ConfigMap", "dynamic", "test", nil)},
			},
			"cross-cluster-forbidden": {
				Input: v1alpha1.ObjectReferrer{
					ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
					ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic", Cluster: "demo"},
				},
				appNs: "test",
				Scope: RefObjectsAvailableScopeCluster,
				Error: "cannot refer to objects outside control plane",
			},
			"test-cluster-scope-resource": {
				Input: v1alpha1.ObjectReferrer{
					ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "clusterrole"},
					ObjectSelector:       v1alpha1.ObjectSelector{Name: "test-cluster-role"},
				},
				appNs:         "test",
				Scope:         RefObjectsAvailableScopeCluster,
				Output:        []*unstructured.Unstructured{createUnstructured("rbac.authorization.k8s.io/v1", "ClusterRole", "test-cluster-role", "", nil)},
				IsClusterRole: true,
			},
		}
		for name, tt := range testcases {
			By("Test " + name)
			if tt.Scope == "" {
				tt.Scope = RefObjectsAvailableScopeGlobal
			}
			RefObjectsAvailableScope = tt.Scope
			output, err := SelectRefObjectsForDispatch(context.Background(), k8sClient, tt.appNs, tt.compName, tt.Input)
			if tt.Error != "" {
				Expect(err).ShouldNot(BeNil())
				Expect(err.Error()).Should(ContainSubstring(tt.Error))
			} else {
				Expect(err).Should(Succeed())
				if tt.IsService {
					Expect(output[0].Object["kind"]).Should(Equal("Service"))
					Expect(output[0].Object["spec"].(map[string]interface{})["clusterIP"]).Should(BeNil())
				} else {
					if tt.IsClusterRole {
						delete(output[0].Object, "rules")
					}
					Expect(output).Should(Equal(tt.Output))
				}
			}
		}
	})

	It("Test AppendUnstructuredObjects", func() {
		testCases := map[string]struct {
			Inputs  []*unstructured.Unstructured
			Input   *unstructured.Unstructured
			Outputs []*unstructured.Unstructured
		}{
			"overlap": {
				Inputs: []*unstructured.Unstructured{{Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "x", "namespace": "default"},
					"data":       "a",
				}}, {Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "y", "namespace": "default"},
					"data":       "b",
				}}},
				Input: &unstructured.Unstructured{Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "y", "namespace": "default"},
					"data":       "c",
				}},
				Outputs: []*unstructured.Unstructured{{Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "x", "namespace": "default"},
					"data":       "a",
				}}, {Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "y", "namespace": "default"},
					"data":       "c",
				}}},
			},
			"append": {
				Inputs: []*unstructured.Unstructured{{Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "x", "namespace": "default"},
					"data":       "a",
				}}, {Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "y", "namespace": "default"},
					"data":       "b",
				}}},
				Input: &unstructured.Unstructured{Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "z", "namespace": "default"},
					"data":       "c",
				}},
				Outputs: []*unstructured.Unstructured{{Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "x", "namespace": "default"},
					"data":       "a",
				}}, {Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "y", "namespace": "default"},
					"data":       "b",
				}}, {Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata":   map[string]interface{}{"name": "z", "namespace": "default"},
					"data":       "c",
				}}},
			},
		}
		for name, tt := range testCases {
			By("Test " + name)
			Expect(AppendUnstructuredObjects(tt.Inputs, tt.Input)).Should(Equal(tt.Outputs))
		}
	})

})
