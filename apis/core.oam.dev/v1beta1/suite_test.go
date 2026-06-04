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

package v1beta1

import (
	"context"
	"log"
	"os"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// testEnv backs the envtest-based admission tests in this package. It is only
// started when the kube-apiserver/etcd binaries are available (via
// KUBEBUILDER_ASSETS / setup-envtest). When they are not, k8sClient stays nil
// and the admission tests skip themselves, so the package's plain unit tests
// still run without the binaries.
var (
	testEnv   *envtest.Environment
	k8sClient client.Client
	testCtx   = context.Background()
)

func TestMain(m *testing.M) {
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{"../../../charts/vela-core/crds"},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		log.Printf("envtest unavailable; admission tests will be skipped: %v", err)
		os.Exit(m.Run())
	}

	testScheme := runtime.NewScheme()
	if err := AddToScheme(testScheme); err != nil {
		log.Fatalf("failed to register scheme: %v", err)
	}
	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	code := m.Run()
	if err := testEnv.Stop(); err != nil {
		log.Printf("failed to stop envtest: %v", err)
	}
	os.Exit(code)
}
