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

package env

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/kubevela/pkg/util/singleton"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/apis/types"
)

var testEnv *envtest.Environment
var cfg *rest.Config
var rawClient client.Client
var testScheme = runtime.NewScheme()

func TestGetOAMHome(t *testing.T) {
	// Save original home value
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	// Test case 1: OAMHOME is set
	os.Setenv("OAMHOME", "/test/oam/home")
	result := GetOAMHome()
	assert.Equal(t, "/test/oam/home", result)

	// Test case 2: OAMHOME is not set, use HOME/.oam
	os.Unsetenv("OAMHOME")
	os.Setenv("HOME", "/test/home")
	result = GetOAMHome()
	assert.Equal(t, "/test/home/.oam", result)
}

func TestGetEnv(t *testing.T) {
	// Test GetEnv with default
	os.Setenv("TEST_VAR", "test-value")
	assert.Equal(t, "test-value", GetEnv("TEST_VAR", "default"))
	assert.Equal(t, "default", GetEnv("NON_EXISTENT_VAR", "default"))
}

func TestGetEnvInt(t *testing.T) {
	// Test GetEnvInt with default
	os.Setenv("TEST_INT", "42")
	assert.Equal(t, 42, GetEnvInt("TEST_INT", 100))
	assert.Equal(t, 100, GetEnvInt("NON_EXISTENT_INT", 100))

	// Test with invalid integer
	os.Setenv("INVALID_INT", "not-an-int")
	assert.Equal(t, 200, GetEnvInt("INVALID_INT", 200))
}

func TestCreateEnv(t *testing.T) {
	// Check if kubebuilder binaries exist
	if _, err := os.Stat("/usr/local/kubebuilder/bin/etcd"); os.IsNotExist(err) {
		t.Skip("Kubebuilder binaries not found, skipping test")
		return
	}

	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths: []string{
			filepath.Join("../../..", "charts/vela-core/crds"), // this has all the required CRDs,
		},
	}
	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		t.Logf("Failed to start test environment: %v", err)
		t.Skip("Environment start failed, skipping test")
		return
	}
	defer func() {
		if testEnv != nil {
			err := testEnv.Stop()
			assert.NoError(t, err)
		}
	}()
	assert.NoError(t, clientgoscheme.AddToScheme(testScheme))

	rawClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	assert.NoError(t, err)

	type want struct {
		data string
	}
	testcases := []struct {
		name    string
		envMeta *types.EnvMeta
		want    want
	}{
		{
			name: "env-application",
			envMeta: &types.EnvMeta{
				Name:      "env-application",
				Namespace: "default",
			},
			want: want{
				data: "",
			},
		},
		{
			name: "default",
			envMeta: &types.EnvMeta{
				Name:      "default",
				Namespace: "default",
			},
			want: want{
				data: "the namespace default was already assigned to env env-application",
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			singleton.KubeClient.Set(rawClient)
			err = CreateEnv(tc.envMeta)
			if err != nil && cmp.Diff(tc.want.data, err.Error()) != "" {
				t.Errorf("CreateEnv(...): \n -want: \n%s,\n +got:\n%s", tc.want.data, err.Error())
			}
		})
	}
}
