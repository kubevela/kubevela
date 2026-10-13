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
	"k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/features"
)

// recorderName is the Event source the sweep's Events carry.
const recorderName = "ConfigMigration"

// Setup adds the startup sweep to the manager when AutoMigrateLegacyConfigs is on
// and does nothing otherwise. It runs through the same loop as the config
// controllers, so the gate decides at startup and nothing is wired for an off gate.
//
// The sweep waits for the webhook server only when the webhooks are enabled
// (args.UseWebhook). mgr.GetWebhookServer is not a plain getter: its first call adds
// the server to the manager, and with --use-webhook=false no certificates are
// mounted, so that server would fail to start and take the manager down with it.
func Setup(mgr ctrl.Manager, args controller.Args) error {
	if !feature.DefaultMutableFeatureGate.Enabled(features.AutoMigrateLegacyConfigs) {
		return nil
	}
	cli, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		return err
	}
	var ready healthz.Checker
	if args.UseWebhook {
		if srv := mgr.GetWebhookServer(); srv != nil {
			ready = srv.StartedChecker()
		}
	}
	klog.InfoS("AutoMigrateLegacyConfigs is on; the legacy config migration sweep will run on the leader", "waitForWebhook", ready != nil)
	return mgr.Add(&StartupSweep{
		Client:   cli,
		Recorder: mgr.GetEventRecorderFor(recorderName),
		Ready:    ready,
	})
}
