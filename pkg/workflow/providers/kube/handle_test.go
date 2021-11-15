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

package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var scheme = runtime.NewScheme()
var pd *packages.PackageDiscover

func TestProvider(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Test Definition Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.BoolPtr(false),
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	Expect(clientgoscheme.AddToScheme(scheme)).Should(BeNil())
	Expect(crdv1.AddToScheme(scheme)).Should(BeNil())
	// +kubebuilder:scaffold:scheme
	By("Create the k8s client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())
	pd, err = packages.NewPackageDiscover(cfg)
	Expect(err).ToNot(HaveOccurred())

	close(done)
}, 120)

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Test Workflow Provider Kube", func() {
	It("apply and read", func() {
		p := &provider{
			apply: func(ctx monitorContext.Context, _ string, _ common.ResourceCreatorRole, manifests ...*unstructured.Unstructured) error {
				for _, obj := range manifests {
					if err := k8sClient.Create(ctx, obj); err != nil {
						if errors.IsAlreadyExists(err) {
							return k8sClient.Update(ctx, obj)
						}
						return err
					}
				}
				return nil
			},
			cli: k8sClient,
		}
		ctx, err := newWorkflowContextForTest()
		Expect(err).ToNot(HaveOccurred())

		component, err := ctx.GetComponent("server")
		Expect(err).ToNot(HaveOccurred())

		v, err := value.NewValue(fmt.Sprintf(`
value:{
	%s
	metadata: name: "app"
}
cluster: ""
`, component.Workload.String()), nil, "")
		Expect(err).ToNot(HaveOccurred())
		err = p.Apply(ctx, nil, v, nil)
		Expect(err).ToNot(HaveOccurred())
		workload, err := component.Workload.Unstructured()
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() error {
			return k8sClient.Get(context.Background(), client.ObjectKey{
				Namespace: "default",
				Name:      "app",
			}, workload)
		}, time.Second*2, time.Millisecond*300).Should(BeNil())

		v, err = value.NewValue(fmt.Sprintf(`
value: {
%s
metadata: name: "app"
}
cluster: ""
`, component.Workload.String()), nil, "")
		Expect(err).ToNot(HaveOccurred())
		err = p.Read(ctx, nil, v, nil)
		Expect(err).ToNot(HaveOccurred())
		result, err := v.LookupValue("value")
		Expect(err).ToNot(HaveOccurred())

		expected := new(unstructured.Unstructured)
		ev, err := result.MakeValue(expectedCue)
		Expect(err).ToNot(HaveOccurred())
		err = ev.UnmarshalTo(expected)
		Expect(err).ToNot(HaveOccurred())

		err = result.FillObject(expected.Object)
		Expect(err).ToNot(HaveOccurred())
	})
	It("patch & apply", func() {
		p := &provider{
			apply: func(ctx monitorContext.Context, _ string, _ common.ResourceCreatorRole, manifests ...*unstructured.Unstructured) error {
				for _, obj := range manifests {
					if err := k8sClient.Create(ctx, obj); err != nil {
						if errors.IsAlreadyExists(err) {
							return k8sClient.Update(ctx, obj)
						}
						return err
					}
				}
				return nil
			},
			cli: k8sClient,
		}
		ctx, err := newWorkflowContextForTest()
		Expect(err).ToNot(HaveOccurred())

		component, err := ctx.GetComponent("server")
		Expect(err).ToNot(HaveOccurred())
		v, err := value.NewValue(fmt.Sprintf(`
value: {%s}
cluster: ""
patch: metadata: name: "test-app-1"`, component.Workload.String()), nil, "")
		Expect(err).ToNot(HaveOccurred())
		err = p.Apply(ctx, nil, v, nil)
		Expect(err).ToNot(HaveOccurred())

		workload, err := component.Workload.Unstructured()
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() error {
			return k8sClient.Get(context.Background(), client.ObjectKey{
				Namespace: "default",
				Name:      "test-app-1",
			}, workload)
		}, time.Second*2, time.Millisecond*300).Should(BeNil())
	})

	It("list", func() {
		p := &provider{
			apply: func(ctx monitorContext.Context, _ string, _ common.ResourceCreatorRole, manifests ...*unstructured.Unstructured) error {
				return nil
			},
			cli: k8sClient,
		}

		ctx := context.Background()
		for i := 2; i >= 0; i-- {
			err := k8sClient.Create(ctx, &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("test-%v", i),
					Namespace: "default",
					Labels: map[string]string{
						"test":  "test",
						"index": fmt.Sprintf("test-%v", i),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  fmt.Sprintf("test-%v", i),
							Image: "busybox",
						},
					},
				},
			})
			Expect(err).ToNot(HaveOccurred())
		}

		By("List pods with labels test=test")
		v, err := value.NewValue(`
resource: {
apiVersion: "v1"
kind: "Pod"
}
filter: {
namespace: "default"
matchingLabels: {
test: "test"
}
}
cluster: ""
`, nil, "")
		Expect(err).ToNot(HaveOccurred())
		wfCtx, err := newWorkflowContextForTest()
		Expect(err).ToNot(HaveOccurred())
		err = p.List(wfCtx, nil, v, nil)
		Expect(err).ToNot(HaveOccurred())
		result, err := v.LookupValue("list")
		Expect(err).ToNot(HaveOccurred())
		expected := &metav1.PartialObjectMetadataList{}
		err = result.UnmarshalTo(expected)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(expected.Items)).Should(Equal(3))

		By("List pods with labels index=test-1")
		v, err = value.NewValue(`
resource: {
apiVersion: "v1"
kind: "Pod"
}
filter: {
matchingLabels: {
index: "test-1"
}
}
cluster: ""
`, nil, "")
		Expect(err).ToNot(HaveOccurred())
		err = p.List(wfCtx, nil, v, nil)
		Expect(err).ToNot(HaveOccurred())
		result, err = v.LookupValue("list")
		Expect(err).ToNot(HaveOccurred())
		expected = &metav1.PartialObjectMetadataList{}
		err = result.UnmarshalTo(expected)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(expected.Items)).Should(Equal(1))
	})

	It("delete", func() {
		p := &provider{
			apply: func(ctx monitorContext.Context, _ string, _ common.ResourceCreatorRole, manifests ...*unstructured.Unstructured) error {
				return nil
			},
			delete: func(ctx context.Context, cluster string, owner common.ResourceCreatorRole, manifest *unstructured.Unstructured) error {
				if err := k8sClient.Delete(ctx, manifest); err != nil {
					return err
				}
				return nil
			},
			cli: k8sClient,
		}

		ctx := context.Background()
		err := k8sClient.Create(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "test",
						Image: "busybox",
					},
				},
			},
		})
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      "test",
			Namespace: "default",
		}, &corev1.Pod{})
		Expect(err).ToNot(HaveOccurred())

		v, err := value.NewValue(`
value: {
apiVersion: "v1"
kind: "Pod"
metadata: {
name: "test"
namespace: "default"
}
}
cluster: ""
`, nil, "")
		Expect(err).ToNot(HaveOccurred())
		wfCtx, err := newWorkflowContextForTest()
		Expect(err).ToNot(HaveOccurred())
		err = p.Delete(wfCtx, nil, v, nil)
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      "test",
			Namespace: "default",
		}, &corev1.Pod{})
		Expect(err).To(HaveOccurred())
		Expect(errors.IsNotFound(err)).Should(Equal(true))
	})

	It("test error case", func() {
		p := &provider{
			apply: func(ctx monitorContext.Context, _ string, _ common.ResourceCreatorRole, manifests ...*unstructured.Unstructured) error {
				for _, obj := range manifests {
					if err := k8sClient.Create(ctx, obj); err != nil {
						if errors.IsAlreadyExists(err) {
							return k8sClient.Update(ctx, obj)
						}
						return err
					}
				}
				return nil
			},
			cli: k8sClient,
		}
		ctx, err := newWorkflowContextForTest()
		Expect(err).ToNot(HaveOccurred())

		v, err := value.NewValue(`
value: {
  kind: "Pod"
  apiVersion: "v1"
  spec: close({kind: 12})	
}`, nil, "")
		Expect(err).ToNot(HaveOccurred())
		err = p.Apply(ctx, nil, v, nil)
		Expect(err).To(HaveOccurred())

		v, _ = value.NewValue(`
value: {
  kind: "Pod"
  apiVersion: "v1"
}
patch: _|_
`, nil, "")
		err = p.Apply(ctx, nil, v, nil)
		Expect(err).To(HaveOccurred())

		v, err = value.NewValue(`
value: {
  metadata: {
     name: "app-xx"
     namespace: "default"
  }
  kind: "Pod"
  apiVersion: "v1"
}
cluster: "test"
`, nil, "")
		Expect(err).ToNot(HaveOccurred())
		err = p.Read(ctx, nil, v, nil)
		Expect(err).ToNot(HaveOccurred())
		errV, err := v.Field("err")
		Expect(err).ToNot(HaveOccurred())
		Expect(errV.Exists()).Should(BeTrue())

		v, err = value.NewValue(`
val: {
  metadata: {
     name: "app-xx"
     namespace: "default"
  }
  kind: "Pod"
  apiVersion: "v1"
}
`, nil, "")
		Expect(err).ToNot(HaveOccurred())
		err = p.Read(ctx, nil, v, nil)
		Expect(err).To(HaveOccurred())
		err = p.Apply(ctx, nil, v, nil)
		Expect(err).To(HaveOccurred())
	})

})

func newWorkflowContextForTest() (wfContext.Context, error) {
	cm := corev1.ConfigMap{}

	testCaseJson, err := yaml.YAMLToJSON([]byte(testCaseYaml))
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(testCaseJson, &cm)
	if err != nil {
		return nil, err
	}

	wfCtx := new(wfContext.WorkflowContext)
	err = wfCtx.LoadFromConfigMap(cm)
	return wfCtx, err
}

var (
	testCaseYaml = `apiVersion: v1
data:
  components: '{"server":"{\"Scopes\":null,\"StandardWorkload\":\"{\\\"apiVersion\\\":\\\"v1\\\",\\\"kind\\\":\\\"Pod\\\",\\\"metadata\\\":{\\\"labels\\\":{\\\"app\\\":\\\"nginx\\\"}},\\\"spec\\\":{\\\"containers\\\":[{\\\"env\\\":[{\\\"name\\\":\\\"APP\\\",\\\"value\\\":\\\"nginx\\\"}],\\\"image\\\":\\\"nginx:1.14.2\\\",\\\"imagePullPolicy\\\":\\\"IfNotPresent\\\",\\\"name\\\":\\\"main\\\",\\\"ports\\\":[{\\\"containerPort\\\":8080,\\\"protocol\\\":\\\"TCP\\\"}]}]}}\",\"Traits\":[\"{\\\"apiVersion\\\":\\\"v1\\\",\\\"kind\\\":\\\"Service\\\",\\\"metadata\\\":{\\\"name\\\":\\\"my-service\\\"},\\\"spec\\\":{\\\"ports\\\":[{\\\"port\\\":80,\\\"protocol\\\":\\\"TCP\\\",\\\"targetPort\\\":8080}],\\\"selector\\\":{\\\"app\\\":\\\"nginx\\\"}}}\"]}"}'
kind: ConfigMap
metadata:
  name: app-v1
`
	expectedCue = `status: {
	phase:    "Pending"
	qosClass: "BestEffort"
}
apiVersion: "v1"
kind:       "Pod"
metadata: {
	name: "app"
	labels: {
		app: "nginx"
	}
	namespace:         "default"
}
spec: {
	containers: [{
		name: "main"
		env: [{
			name:  "APP"
			value: "nginx"
		}]
		image:           "nginx:1.14.2"
		imagePullPolicy: "IfNotPresent"
		ports: [{
			containerPort: 8080
			protocol:      "TCP"
		}]
		resources: {}
		terminationMessagePath:   "/dev/termination-log"
		terminationMessagePolicy: "File"
	}]
	dnsPolicy:          "ClusterFirst"
	enableServiceLinks: true
	preemptionPolicy:   "PreemptLowerPriority"
	priority:           0
	restartPolicy:      "Always"
	schedulerName:      "default-scheduler"
	securityContext: {}
	terminationGracePeriodSeconds: 30
	tolerations: [{
		effect:            "NoExecute"
		key:               "node.kubernetes.io/not-ready"
		operator:          "Exists"
		tolerationSeconds: 300
	}, {
		effect:            "NoExecute"
		key:               "node.kubernetes.io/unreachable"
		operator:          "Exists"
		tolerationSeconds: 300
	}]
}`
)
