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

package main

import (
	"flag"
	"os"

	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	core_oam_dev "github.com/oam-dev/kubevela/apis/core.oam.dev"
	oamstandard "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	oamcontroller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/controller/standard.oam.dev/v1alpha1/rollout"
	"github.com/oam-dev/kubevela/pkg/dependency/kruiseapi"
	"github.com/oam-dev/kubevela/version"
)

var (
	scheme = k8sruntime.NewScheme()
)

const (
	kubevelaRuntimeRolloutName = "kubevela-runtime-rollout"
)

func init() {
	_ = oamstandard.AddToScheme(scheme)
	// need request resourceTracker
	_ = core_oam_dev.AddToScheme(scheme)
	// need request controllerRevision and deployment
	_ = clientgoscheme.AddToScheme(scheme)
	// need request cloneset
	_ = kruiseapi.AddToScheme(scheme)
}

func main() {
	var controllerArgs oamcontroller.Args
	var leaderElectionNamespace string
	var useWebhook bool
	var enableLeaderElection bool
	var healthAddr string

	flag.BoolVar(&useWebhook, "use-webhook", false, "Enable Admission Webhook")
	flag.IntVar(&controllerArgs.ConcurrentReconciles, "concurrent-reconciles", 4, "concurrent-reconciles is the concurrent reconcile number of the controller. The default value is 4")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "vela-std",
		"Determines the namespace in which the leader election configmap will be created.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&healthAddr, "health-addr", ":19440", "The address the health endpoint binds to.")
	flag.Parse()

	// setup logging
	klog.InitFlags(nil)

	restConfig := ctrl.GetConfigOrDie()
	restConfig.UserAgent = kubevelaRuntimeRolloutName + "/" + version.GitRevision

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                  scheme,
		LeaderElectionNamespace: leaderElectionNamespace,
		LeaderElectionID:        kubevelaRuntimeRolloutName,
		HealthProbeBindAddress:  healthAddr,
	})
	if err != nil {
		klog.ErrorS(err, "Unable to create a controller manager")
		os.Exit(1)
	}

	klog.Info("Create readiness/health check")
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		klog.ErrorS(err, "Unable to register ready check")
		return
	}
	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		klog.ErrorS(err, "Unable to register healthy check")
		return
	}

	var functions []func(ctrl.Manager, oamcontroller.Args) error
	functions = append(functions, rollout.Setup)
	for _, setup := range functions {
		if err := setup(mgr, controllerArgs); err != nil {
			klog.ErrorS(err, "Unable to setup the vela runtime controller")
			os.Exit(1)
		}
	}

	klog.Info("Start the vela runtime controller manager")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		klog.ErrorS(err, "Failed to run manager")
		os.Exit(1)
	}
	klog.Info("Safely stops Program...")
}
