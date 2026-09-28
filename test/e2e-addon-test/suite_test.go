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
	"os/exec"
	"path/filepath"
	sysruntime "runtime"
	"testing"
	"time"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	terraformv1beta1 "github.com/oam-dev/terraform-controller/api/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	// +kubebuilder:scaffold:imports
)

var k8sClient client.Client
var scheme = runtime.NewScheme()

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Addons Controller Suite")
}

func initializeAddonClient() {
	if k8sClient != nil {
		return
	}
	By("Bootstrapping test environment")
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	err := clientgoscheme.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = core.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = crdv1.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = terraformv1beta1.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = workflowv1alpha1.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	By("Setting up kubernetes client")
	k8sClient, err = client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		logf.Log.Error(err, "failed to create k8sClient")
		Fail("setup failed")
	}
	By("Finished setting up test environment")
}

var _ = SynchronizedBeforeSuite(func() {
	initializeAddonClient()
	// Both addon installations change shared controllers, CRDs and RBAC. Finish
	// them before independent workload specs are scheduled to any worker.
	By("Install Addon Terraform")
	prepareAddonCleanup("terraform-alibaba")
	enableAddonForSuite("terraform-alibaba")
	By("Install Addon Workflow")
	prepareAddonCleanup("vela-workflow")
	enableAddonForSuite("vela-workflow")
}, func() {
	initializeAddonClient()
})

func enableAddonForSuite(name string, args ...string) {
	output, err := runVelaAddonCommand(append([]string{"addon", "enable", name}, args...)...)
	Expect(err).To(Succeed(), "vela addon enable %s failed: %s", name, output)
	Expect(string(output)).To(ContainSubstring("enabled successfully"))
}

func prepareAddonCleanup(name string) {
	existed, err := addonApplicationExists(context.Background(), k8sClient, name)
	Expect(err).To(Succeed())
	if existed {
		return
	}
	DeferCleanup(func() {
		exists, err := addonApplicationExists(context.Background(), k8sClient, name)
		Expect(err).To(Succeed())
		if !exists {
			return
		}
		output, err := runVelaAddonCommand("addon", "disable", name, "--yes")
		Expect(err).To(Succeed(), "clean up addon %s: %s", name, output)
	})
}

func addonApplicationExists(ctx context.Context, cli client.Client, name string) (bool, error) {
	var app v1beta1.Application
	err := cli.Get(ctx, client.ObjectKey{Name: "addon-" + name, Namespace: "vela-system"}, &app)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func runVelaAddonCommand(args ...string) ([]byte, error) {
	_, sourceFile, _, ok := sysruntime.Caller(0)
	Expect(ok).To(BeTrue())
	repoRoot := filepath.Join(filepath.Dir(sourceFile), "..", "..")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(repoRoot, "bin", "vela"), args...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	GinkgoWriter.Printf("vela %v: %s\n", args, output)
	return output, err
}
