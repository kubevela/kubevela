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

package framework

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	controllerscheme "sigs.k8s.io/controller-runtime/pkg/scheme"

	configoam "github.com/oam-dev/kubevela/apis/config.oam.dev"
	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	// +kubebuilder:scaffold:imports
)

// Framework owns one test process's client and suite lifecycle.
type Framework struct {
	Client           client.Client
	Scheme           *runtime.Scheme
	authSetupStarted bool
}

func New() *Framework { return &Framework{Scheme: runtime.NewScheme()} }

func (f *Framework) bootstrap() {
	if f.Client != nil {
		return
	}
	By("Bootstrapping test environment")
	rand.Seed(time.Now().UnixNano())
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	err := clientgoscheme.AddToScheme(f.Scheme)
	Expect(err).Should(BeNil())
	err = core.AddToScheme(f.Scheme)
	Expect(err).Should(BeNil())
	err = crdv1.AddToScheme(f.Scheme)
	Expect(err).Should(BeNil())
	err = kruise.AddToScheme(f.Scheme)
	Expect(err).Should(BeNil())
	err = configoam.AddToScheme(f.Scheme)
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
	err = depSchemeBuilder.AddToScheme(f.Scheme)
	Expect(err).Should(BeNil())
	By("Setting up kubernetes client")
	f.Client, err = client.New(config.GetConfigOrDie(), client.Options{Scheme: f.Scheme})
	if err != nil {
		logf.Log.Error(err, "failed to create Kubernetes client")
		Fail("setup failed")
	}
	By("Finished setting up test environment")
}

// Register synchronizes shared setup and waits for all workers before cleanup.
func (f *Framework) Register(auth bool, ready func(client.Client)) bool {
	SynchronizedBeforeSuite(func() {
		f.bootstrap()
		ready(f.Client)

		// create workload definition for 'deployments'
		wdDeploy := v1beta1.WorkloadDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "deployments.apps",
				Namespace: "vela-system",
			},
			Spec: v1beta1.WorkloadDefinitionSpec{
				Reference: commontypes.DefinitionReference{
					Name: "deployments.apps",
				},
			},
		}
		Expect(f.Client.Create(context.Background(), &wdDeploy)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		By("Created deployments.apps")

		var token [12]byte
		_, err := cryptorand.Read(token[:])
		Expect(err).NotTo(HaveOccurred())
		runID := hex.EncodeToString(token[:])
		ownedRBAC, err := installCoreSuiteRBAC(context.Background(), f.Client, runID)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(ownedRBAC.cleanup(context.Background(), f.Client)).To(Succeed())
		})
		By("Created example.com cluster role and binding for the test service account")

		if auth && os.Getenv("KUBEVELA_E2E_AUTH") == "1" {
			var ns corev1.Namespace
			err := f.Client.Get(context.Background(), client.ObjectKey{Name: authTestNamespace}, &ns)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "auth-test namespace already exists or could not be checked: %v", err)
			f.authSetupStarted = true
			By("Bringing up auth-test registries")
			Expect(setupAuthRegistries(context.Background(), f.Client)).To(Succeed())

			By("Pushing test chart to auth-test registries")
			cfg, err := authTestRestConfig()
			Expect(err).NotTo(HaveOccurred())
			Expect(pushTestChartToRegistries(context.Background(), cfg)).To(Succeed())
		} else {
			By("Skipping auth-test registries setup for this suite")
		}
		f.waitForControllerReconciling(context.Background())

	}, func() { f.bootstrap(); ready(f.Client) })
	SynchronizedAfterSuite(func() {}, func() {
		By("Tearing down the test environment")
		if f.authSetupStarted {
			By("Tearing down auth-test registries")
			Expect(tearDownAuthRegistries(context.Background(), f.Client)).To(Succeed())
		}
	})
	return true
}

// waitForControllerReconciling waits until the controller runs an Application
// to completion. A vela-core Deployment can report Available while its new pod
// still waits on the leader lease, and the auth setup restarts it to inject a
// CA, so without this the first spec races the controller's start.
func (f *Framework) waitForControllerReconciling(ctx context.Context) {
	By("Waiting for the controller to reconcile a canary Application")
	name := RandomNamespaceName("e2e-canary")
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: v1beta1.ApplicationSpec{Components: []commontypes.ApplicationComponent{{
			Name:       "canary",
			Type:       "k8s-objects",
			Properties: &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":%q}}]}`, name))},
		}}},
	}
	Eventually(func() error { return f.Client.Create(ctx, app) }, 30*time.Second, time.Second).Should(Succeed())
	DeferCleanup(func() { _ = f.Client.Delete(ctx, app) })
	Eventually(func(g Gomega) {
		current := &v1beta1.Application{}
		g.Expect(f.Client.Get(ctx, client.ObjectKeyFromObject(app), current)).To(Succeed())
		g.Expect(current.Status.Phase).To(Equal(commontypes.ApplicationRunning))
	}, 2*time.Minute, 500*time.Millisecond).Should(Succeed())
}

// RequestReconcileNow queues o for an immediate reconcile. The Application
// controller watches neither Component nor Trait definitions, nor the
// workloads it applies, so a change to them is otherwise seen on the next
// resync.
func (f *Framework) RequestReconcileNow(ctx context.Context, o client.Object) {
	By(fmt.Sprintf("Request reconcile %q now", o.GetName()))
	_, err := f.requestReconcile(ctx, o)
	Expect(err).Should(Succeed())
}

// requestReconcile stamps an annotation, which passes the Application
// controller's update predicate, and returns the resulting resourceVersion.
// It patches metadata only, so a stale o cannot overwrite the spec.
func (f *Framework) requestReconcile(ctx context.Context, o client.Object) (string, error) {
	patch := fmt.Sprintf(`{"metadata":{"annotations":{"app.oam.dev/requestreconcile":%q}}}`, time.Now().Format(time.RFC3339Nano))
	patched := o.DeepCopyObject().(client.Object)
	if err := f.Client.Patch(ctx, patched, client.RawPatch(types.MergePatchType, []byte(patch))); err != nil {
		return "", err
	}
	return patched.GetResourceVersion(), nil
}

// reconcileRequester requests reconciles of one object only while the
// controller leaves it alone. A request that lands while a reconcile is in
// flight makes that reconcile's status write conflict, and its progress, such
// as a failing step's retry count, is lost. The controller writes the object
// while a busy app requeues itself, so a request waits until the object has
// been unchanged for quietPeriod: the controller is then waiting out a backoff
// or the resync, which is the wait a request is meant to cut short.
type reconcileRequester struct {
	framework *Framework
	ctx       context.Context
	obj       client.Object
	seenRV    string
	seenAt    time.Time
}

const quietPeriod = 3 * time.Second

func (r *reconcileRequester) request(g Gomega) {
	current := r.obj.DeepCopyObject().(client.Object)
	g.Expect(r.framework.Client.Get(r.ctx, client.ObjectKeyFromObject(r.obj), current)).To(Succeed())
	now := time.Now()
	if rv := current.GetResourceVersion(); rv != r.seenRV {
		r.seenRV, r.seenAt = rv, now
	}
	if now.Sub(r.seenAt) < quietPeriod {
		return
	}
	rv, err := r.framework.requestReconcile(r.ctx, r.obj)
	g.Expect(err).To(Succeed())
	r.seenRV, r.seenAt = rv, now
}

// EventuallyReconciled polls assertion, requesting a reconcile of o whenever the
// controller has left it alone for quietPeriod. Every reconcile also re-runs a
// failing workflow step, so the long gaps of the step backoff are cut short.
func (f *Framework) EventuallyReconciled(ctx context.Context, o client.Object, assertion func(g Gomega)) AsyncAssertion {
	r := &reconcileRequester{framework: f, ctx: ctx, obj: o}
	return Eventually(func(g Gomega) {
		r.request(g)
		assertion(g)
	}).WithPolling(time.Second).WithTimeout(2 * time.Minute)
}

// ConsistentlyReconciled holds assertion while requesting reconciles of o, so
// "nothing changed" is checked across real reconciles. The controller records
// nothing when a requested reconcile completes, so the window spans three
// requests rather than relying on the first one finishing.
func (f *Framework) ConsistentlyReconciled(ctx context.Context, o client.Object, assertion func(g Gomega)) AsyncAssertion {
	r := &reconcileRequester{framework: f, ctx: ctx, obj: o}
	return Consistently(func(g Gomega) {
		r.request(g)
		assertion(g)
	}).WithPolling(time.Second).WithTimeout(4 * quietPeriod)
}

// RandomNamespaceName generates an independent DNS-label name for each case.
// Empty prefixes are used as suffixes by the PostDispatch fixtures; keep that
// suffix short enough for their longest object name.
func RandomNamespaceName(basic string) string {
	var token [8]byte
	if _, err := cryptorand.Read(token[:]); err != nil {
		panic(fmt.Errorf("generate test name: %w", err))
	}
	suffix := "-" + hex.EncodeToString(token[:])
	if basic == "" {
		return suffix
	}
	worker := fmt.Sprintf("-p%d", GinkgoParallelProcess())
	suffix = worker + suffix
	var readable strings.Builder
	lastWasSeparator := false
	for _, char := range strings.ToLower(basic) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			readable.WriteRune(char)
			lastWasSeparator = false
		} else if readable.Len() > 0 && !lastWasSeparator {
			readable.WriteByte('-')
			lastWasSeparator = true
		}
	}
	prefix := strings.Trim(readable.String(), "-")
	if prefix == "" {
		prefix = "e2e"
	}
	if len(prefix) > 63-len(suffix) {
		prefix = strings.TrimRight(prefix[:63-len(suffix)], "-")
	}
	return prefix + suffix
}
