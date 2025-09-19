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

package appfile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"

	coreoam "github.com/oam-dev/kubevela/apis/core.oam.dev"
	corev1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

var cfg *rest.Config
var scheme *runtime.Scheme
var k8sClient client.Client
var testEnv *envtest.Environment
var definitionDir string
var wd corev1beta1.WorkloadDefinition
var addonNamespace = "test-addon"

func TestMain(m *testing.M) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	ctx := context.Background()

	useExistCluster := false
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths:        []string{filepath.Join("..", "..", "charts", "vela-core", "crds")},
		UseExistingCluster:       &useExistCluster,
	}

	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start test environment: %v\n", err)
		os.Exit(1)
	}

	// cleanupAndExit stops the test environment and exits with the given code.
	cleanupAndExit := func(code int) {
		// Clean up other resources before stopping the environment
		if k8sClient != nil {
			_ = k8sClient.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: addonNamespace}})
			_ = k8sClient.Delete(context.Background(), &wd)
		}
		if err := testEnv.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to stop test environment: %v\n", err)
		}
		os.Exit(code)
	}

	scheme = runtime.NewScheme()
	if err := coreoam.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to add coreoam to scheme: %v\n", err)
		cleanupAndExit(1)
	}
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to add clientgoscheme to scheme: %v\n", err)
		cleanupAndExit(1)
	}
	if err := crdv1.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to add crdv1 to scheme: %v\n", err)
		cleanupAndExit(1)
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create k8sClient: %v\n", err)
		cleanupAndExit(1)
	}

	definitionDir, err = system.GetCapabilityDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get capability dir: %v\n", err)
		cleanupAndExit(1)
	}
	if err := os.MkdirAll(definitionDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create capability dir: %v\n", err)
		cleanupAndExit(1)
	}

	if err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: addonNamespace}}); err != nil {
		if !errors.IsAlreadyExists(err) {
			fmt.Fprintf(os.Stderr, "Failed to create test namespace: %v\n", err)
			cleanupAndExit(1)
		}
	}

	workloadData, err := os.ReadFile("testdata/workloadDef.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read workloadDef.yaml: %v\n", err)
		cleanupAndExit(1)
	}

	if err := yaml.Unmarshal(workloadData, &wd); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to unmarshal workloadDef.yaml: %v\n", err)
		cleanupAndExit(1)
	}

	wd.Namespace = addonNamespace
	if err := k8sClient.Create(ctx, &wd); err != nil {
		if !errors.IsAlreadyExists(err) {
			fmt.Fprintf(os.Stderr, "Failed to create workload definition: %v\n", err)
			cleanupAndExit(1)
		}
	}

	def, err := os.ReadFile("testdata/terraform-aliyun-oss-workloadDefinition.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read terraform-aliyun-oss-workloadDefinition.yaml: %v\n", err)
		cleanupAndExit(1)
	}
	var terraformDefinition corev1beta1.WorkloadDefinition
	if err := yaml.Unmarshal(def, &terraformDefinition); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to unmarshal terraformDefinition: %v\n", err)
		cleanupAndExit(1)
	}
	terraformDefinition.Namespace = addonNamespace
	if err := k8sClient.Create(ctx, &terraformDefinition); err != nil {
		if !errors.IsAlreadyExists(err) {
			fmt.Fprintf(os.Stderr, "Failed to create terraform workload definition: %v\n", err)
			cleanupAndExit(1)
		}
	}

	code := m.Run()
	cleanupAndExit(code)
}
