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

package component

import (
	"log"
	"math/rand"
	"os"
	"testing"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	pkgcommon "github.com/oam-dev/kubevela/pkg/utils/common"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestMain(m *testing.M) {
	rand.Seed(time.Now().UnixNano())

	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       ptr.To(false),
		CRDDirectoryPaths:        []string{"./testdata"},
	}

	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		log.Fatalf("Failed to start test environment: %v", err)
	}

	cfg.Timeout = time.Minute * 2
	k8sClient, err = client.New(cfg, client.Options{Scheme: pkgcommon.Scheme})
	if err != nil {
		log.Fatalf("Failed to create new kube client: %v", err)
	}

	code := m.Run()

	if err = testEnv.Stop(); err != nil {
		log.Printf("Failed to tear down the test environment: %v", err)
	}
	os.Exit(code)
}
