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

package migration

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/features"
)

// recordingManager is the slice of manager.Manager that Setup touches. Everything
// else panics through the nil embedded interface, which is what we want.
type recordingManager struct {
	manager.Manager
	webhook webhook.Server
	// webhookAsked counts GetWebhookServer calls; in controller-runtime the first one adds
	// the server to the manager, which then starts it
	webhookAsked int
	added        []manager.Runnable
}

// stubWebhookServer is the slice of webhook.Server that Setup touches: a server
// whose StartedChecker passes at once.
type stubWebhookServer struct{ webhook.Server }

func (stubWebhookServer) StartedChecker() healthz.Checker { return healthz.Ping }

func (m *recordingManager) GetConfig() *rest.Config    { return cfg }
func (m *recordingManager) GetScheme() *runtime.Scheme { return scheme.Scheme }
func (m *recordingManager) GetWebhookServer() webhook.Server {
	m.webhookAsked++
	return m.webhook
}
func (m *recordingManager) GetEventRecorderFor(string) record.EventRecorder {
	return record.NewFakeRecorder(1)
}
func (m *recordingManager) Add(r manager.Runnable) error {
	m.added = append(m.added, r)
	return nil
}

var _ = Describe("Setup of the startup migration sweep", func() {
	It("registers nothing while the gate is off", func() {
		featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultMutableFeatureGate, features.AutoMigrateLegacyConfigs, false)
		mgr := &recordingManager{}
		Expect(Setup(mgr, controller.Args{})).Should(Succeed())
		Expect(mgr.added).Should(BeEmpty())
	})

	It("registers one leader-elected sweep when the gate is on", func() {
		featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultMutableFeatureGate, features.AutoMigrateLegacyConfigs, true)
		mgr := &recordingManager{}
		Expect(Setup(mgr, controller.Args{UseWebhook: true})).Should(Succeed())
		Expect(mgr.added).Should(HaveLen(1))
		sweep, ok := mgr.added[0].(*StartupSweep)
		Expect(ok).Should(BeTrue())
		Expect(sweep.NeedLeaderElection()).Should(BeTrue())
		Expect(sweep.Client).ShouldNot(BeNil())
		Expect(sweep.Recorder).ShouldNot(BeNil())
		Expect(sweep.Ready).Should(BeNil(), "no webhook server on this manager")
	})

	It("hands the sweep the webhook server's started checker when the webhooks are enabled", func() {
		featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultMutableFeatureGate, features.AutoMigrateLegacyConfigs, true)
		mgr := &recordingManager{webhook: stubWebhookServer{}}
		Expect(Setup(mgr, controller.Args{UseWebhook: true})).Should(Succeed())
		Expect(mgr.added).Should(HaveLen(1))
		sweep := mgr.added[0].(*StartupSweep)
		Expect(mgr.webhookAsked).Should(Equal(1))
		Expect(sweep.Ready).ShouldNot(BeNil())
		Expect(sweep.Ready(&http.Request{})).Should(Succeed())
	})

	It("never asks for the webhook server when the webhooks are disabled", func() {
		// The manager always carries a webhook server, enabled or not. Asking for it
		// would add it to the manager, and with --use-webhook=false there are no
		// certificates for it to start with.
		featuregatetesting.SetFeatureGateDuringTest(GinkgoT(), utilfeature.DefaultMutableFeatureGate, features.AutoMigrateLegacyConfigs, true)
		mgr := &recordingManager{webhook: stubWebhookServer{}}
		Expect(Setup(mgr, controller.Args{UseWebhook: false})).Should(Succeed())
		Expect(mgr.added).Should(HaveLen(1))
		sweep := mgr.added[0].(*StartupSweep)
		Expect(mgr.webhookAsked).Should(BeZero())
		Expect(sweep.Ready).Should(BeNil())
	})
})
