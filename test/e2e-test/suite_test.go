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
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	rbac "k8s.io/api/rbac/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	controllerscheme "sigs.k8s.io/controller-runtime/pkg/scheme"

	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	// +kubebuilder:scaffold:imports
)

var k8sClient client.Client
var scheme = runtime.NewScheme()
var roleName = "oam-example-com"
var roleBindingName = "oam-role-binding"

var (
	noNetworkingV1 bool
)

// A DefinitionExtension is an Object type for xxxDefinitin.spec.extension
type DefinitionExtension struct {
	Alias string `json:"alias,omitempty"`
}

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "OAM Core Resource Controller Suite")
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	rand.Seed(time.Now().UnixNano())
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	err := clientgoscheme.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = core.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = crdv1.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = kruise.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	depExample := &unstructured.Unstructured{}
	depExample.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "example.com",
		Version: "v1",
		Kind:    "Foo",
	})
	depSchemeGroupVersion := schema.GroupVersion{Group: "example.com", Version: "v1"}
	depSchemeBuilder := &controllerscheme.Builder{GroupVersion: depSchemeGroupVersion}
	depSchemeBuilder.Register(depExample.DeepCopyObject())
	err = depSchemeBuilder.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = v1alpha1.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	By("Setting up kubernetes client")
	k8sClient, err = client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		logf.Log.Error(err, "failed to create k8sClient")
		Fail("setup failed")
	}
	By("Finished setting up test environment")

	// create workload definition for 'deployments'
	wdDeploy := v1alpha2.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deployments.apps",
			Namespace: "vela-system",
		},
		Spec: v1alpha2.WorkloadDefinitionSpec{
			Reference: commontypes.DefinitionReference{
				Name: "deployments.apps",
			},
		},
	}
	Expect(k8sClient.Create(context.Background(), &wdDeploy)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	By("Created deployments.apps")

	exampleClusterRole := rbac.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: roleName,
			Labels: map[string]string{
				"oam":                                  "clusterrole",
				"rbac.oam.dev/aggregate-to-controller": "true",
			},
		},
		Rules: []rbac.PolicyRule{{
			APIGroups: []string{"example.com"},
			Resources: []string{rbac.ResourceAll},
			Verbs:     []string{rbac.VerbAll},
		}},
	}
	Expect(k8sClient.Create(context.Background(), &exampleClusterRole)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	By("Created example.com cluster role for the test service account")

	adminRoleBinding := rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   roleBindingName,
			Labels: map[string]string{"oam": "clusterrole"},
		},
		Subjects: []rbac.Subject{
			{
				Kind: "User",
				Name: "system:serviceaccount:oam-system:oam-kubernetes-runtime-e2e",
			},
		},
		RoleRef: rbac.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
	}
	Expect(k8sClient.Create(context.Background(), &adminRoleBinding)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	By("Created cluster role binding for the test service account")

	close(done)
}, 300)

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	adminRoleBinding := rbac.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   roleBindingName,
			Labels: map[string]string{"oam": "clusterrole"},
		},
	}
	Expect(k8sClient.Delete(context.Background(), &adminRoleBinding)).Should(BeNil())
	By("Deleted the cluster role binding")
})

// RequestReconcileNow will trigger an immediate reconciliation on K8s object.
// Some test cases may fail for timeout to wait a scheduled reconciliation.
// This is a workaround to avoid long-time wait before next scheduled
// reconciliation.
func RequestReconcileNow(ctx context.Context, o client.Object) {
	oCopy := o.DeepCopyObject()
	oMeta, ok := oCopy.(metav1.Object)
	Expect(ok).Should(BeTrue())
	oMeta.SetAnnotations(map[string]string{
		"app.oam.dev/requestreconcile": time.Now().String(),
	})
	oMeta.SetResourceVersion("")
	By(fmt.Sprintf("Requset reconcile %q now", oMeta.GetName()))
	Expect(k8sClient.Patch(ctx, oCopy.(client.Object), client.Merge)).Should(Succeed())
}

// randomNamespaceName generates a random name based on the basic name.
// Running each ginkgo case in a new namespace with a random name can avoid
// waiting a long time to GC namespace.
func randomNamespaceName(basic string) string {
	return fmt.Sprintf("%s-%s", basic, strconv.FormatInt(rand.Int63(), 16))
}
