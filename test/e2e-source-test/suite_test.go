/*
Copyright 2026 The KubeVela Authors.

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

package source_test

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	configoam "github.com/oam-dev/kubevela/apis/config.oam.dev"
	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var k8sClient client.Client
var scheme = runtime.NewScheme()

func TestSourceE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Source Definition E2E Suite")
}

func initializeSourceClient() {
	if k8sClient != nil {
		return
	}
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(core.AddToScheme(scheme)).To(Succeed())
	Expect(configoam.AddToScheme(scheme)).To(Succeed())
	var err error
	k8sClient, err = client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	Expect(err).To(Succeed())
}

var _ = SynchronizedBeforeSuite(func() {
	initializeSourceClient()
	waitForSourceController(context.Background())
}, func() {
	initializeSourceClient()
})

func waitForSourceController(ctx context.Context) {
	name := randomNamespaceName("source-canary")
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: v1beta1.ApplicationSpec{Components: []oamcomm.ApplicationComponent{{
			Name: "canary", Type: "k8s-objects",
			Properties: &runtime.RawExtension{Raw: []byte(fmt.Sprintf(`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":%q}}]}`, name))},
		}}},
	}
	Expect(k8sClient.Create(ctx, app)).To(Succeed())
	DeferCleanup(func() { Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, app))).To(Succeed()) })
	Eventually(func(g Gomega) {
		current := &v1beta1.Application{}
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), current)).To(Succeed())
		g.Expect(current.Status.Phase).To(Equal(oamcomm.ApplicationRunning))
	}, 2*time.Minute, 500*time.Millisecond).Should(Succeed())
}

func createNamespace(ctx context.Context, name string) corev1.Namespace {
	ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	Expect(k8sClient.Create(ctx, &ns)).To(Succeed())
	return ns
}

func verifyApplicationPhase(ctx context.Context, namespace, name string, phase oamcomm.ApplicationPhase) {
	Eventually(func() error {
		app := &v1beta1.Application{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, app); err != nil {
			return err
		}
		if app.Status.Phase != phase {
			return fmt.Errorf("application status wants %s, actually %s", phase, app.Status.Phase)
		}
		return nil
	}, 120*time.Second, time.Second).Should(Succeed())
}

func randomNamespaceName(prefix string) string {
	var token [8]byte
	if _, err := cryptorand.Read(token[:]); err != nil {
		panic(fmt.Errorf("generate source test name: %w", err))
	}
	suffix := fmt.Sprintf("-p%d-%s", GinkgoParallelProcess(), hex.EncodeToString(token[:]))
	readable := strings.Trim(strings.ToLower(prefix), "-")
	if readable == "" {
		readable = "source-e2e"
	}
	if len(readable) > 63-len(suffix) {
		readable = strings.TrimRight(readable[:63-len(suffix)], "-")
	}
	return readable + suffix
}
