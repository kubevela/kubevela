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
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kubevela/pkg/util/singleton"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

var cfg *rest.Config
var k8sClient client.Client
var testScheme = runtime.NewScheme()

func TestMain(m *testing.M) {
	testEnv := &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths: []string{
			filepath.Join("../../..", "charts/vela-core/crds"),
		},
	}

	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		panic(err)
	}

	if err := clientgoscheme.AddToScheme(testScheme); err != nil {
		panic(err)
	}
	if err := v1beta1.AddToScheme(testScheme); err != nil {
		panic(err)
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	if err != nil {
		panic(err)
	}
	singleton.KubeClient.Set(k8sClient)

	code := m.Run()

	if err := testEnv.Stop(); err != nil {
		panic(err)
	}

	os.Exit(code)
}

func createTestNamespace(ctx context.Context, t *testing.T, nsName string) {
	ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
	err := k8sClient.Create(ctx, ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		assert.NoError(t, err)
	}
}

func TestCreateEnv(t *testing.T) {
	ctx := context.Background()
	t.Run("create a new env successfully", func(t *testing.T) {
		nsName := "create-env-ns-1"
		createTestNamespace(ctx, t, nsName)
		envMeta := &types.EnvMeta{Name: "env-create-1", Namespace: nsName}
		err := CreateEnv(envMeta)
		assert.NoError(t, err)

		var createdNs v1.Namespace
		err = k8sClient.Get(ctx, client.ObjectKey{Name: nsName}, &createdNs)
		assert.NoError(t, err)
		assert.Equal(t, "env-create-1", createdNs.Labels[oam.LabelNamespaceOfEnvName])
		assert.Equal(t, oam.VelaNamespaceUsageEnv, createdNs.Labels[oam.LabelControlPlaneNamespaceUsage])
	})

	t.Run("create env with a name that already exists", func(t *testing.T) {
		nsName1, nsName2 := "create-env-ns-2", "create-env-ns-3"
		createTestNamespace(ctx, t, nsName1)
		createTestNamespace(ctx, t, nsName2)

		envMeta1 := &types.EnvMeta{Name: "env-exist", Namespace: nsName1}
		err := CreateEnv(envMeta1)
		assert.NoError(t, err)

		envMeta2 := &types.EnvMeta{Name: "env-exist", Namespace: nsName2}
		err = CreateEnv(envMeta2)
		assert.Error(t, err)
		assert.Equal(t, "env name env-exist already exists", err.Error())
	})

	t.Run("create env in a namespace that is already used by another env", func(t *testing.T) {
		nsName := "create-env-ns-4"
		createTestNamespace(ctx, t, nsName)

		envMeta1 := &types.EnvMeta{Name: "env-ns-used-1", Namespace: nsName}
		err := CreateEnv(envMeta1)
		assert.NoError(t, err)

		envMeta2 := &types.EnvMeta{Name: "env-ns-used-2", Namespace: nsName}
		err = CreateEnv(envMeta2)
		assert.Error(t, err)
		assert.Equal(t, "the namespace create-env-ns-4 was already assigned to env env-ns-used-1", err.Error())
	})
}

func TestGetEnvByName(t *testing.T) {
	ctx := context.Background()
	nsName := "get-env-ns"
	envName := "test-get"
	createTestNamespace(ctx, t, nsName)
	assert.NoError(t, CreateEnv(&types.EnvMeta{Name: envName, Namespace: nsName}))

	t.Run("get existing env", func(t *testing.T) {
		env, err := GetEnvByName(envName)
		assert.NoError(t, err)
		assert.Equal(t, envName, env.Name)
		assert.Equal(t, nsName, env.Namespace)
	})

	t.Run("get default env", func(t *testing.T) {
		env, err := GetEnvByName("default")
		assert.NoError(t, err)
		assert.Equal(t, "default", env.Name)
		assert.Equal(t, "default", env.Namespace)
	})

	t.Run("get non-existent env", func(t *testing.T) {
		_, err := GetEnvByName("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not exist")
	})
}

func TestListAndCurrentEnvs(t *testing.T) {
	ctx := context.Background()
	// Setup a temp home for current env file
	tmpDir, err := os.MkdirTemp("", "vela-home")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	t.Setenv("HOME", tmpDir)

	// Create test envs
	nsList1, envList1 := "list-env-ns-1", "test-list-1"
	createTestNamespace(ctx, t, nsList1)
	assert.NoError(t, CreateEnv(&types.EnvMeta{Name: envList1, Namespace: nsList1}))

	nsList2, envList2 := "list-env-ns-2", "test-list-2"
	createTestNamespace(ctx, t, nsList2)
	assert.NoError(t, CreateEnv(&types.EnvMeta{Name: envList2, Namespace: nsList2}))

	t.Run("list specific env", func(t *testing.T) {
		envs, err := ListEnvs(envList1)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(envs))
		assert.Equal(t, envList1, envs[0].Name)
	})

	t.Run("list non-existent env", func(t *testing.T) {
		_, err := ListEnvs("non-existent-list")
		assert.Error(t, err)
	})

	t.Run("list all envs and check current marker", func(t *testing.T) {
		envs, err := ListEnvs("")
		assert.NoError(t, err)
		found1, found2, current := false, false, false
		for _, e := range envs {
			if e.Name == envList1 {
				found1 = true
			}
			if e.Name == envList2 {
				found2 = true
			}
			if e.Current == "*" {
				current = true
			}
		}
		assert.True(t, found1)
		assert.True(t, found2)
		assert.True(t, current) // ListEnvs should set a default current env
	})

	t.Run("set and get current env", func(t *testing.T) {
		testEnvForCurrent := &types.EnvMeta{Name: envList2, Namespace: nsList2}
		err := SetCurrentEnv(testEnvForCurrent)
		assert.NoError(t, err)

		curEnv, err := GetCurrentEnv()
		assert.NoError(t, err)
		assert.Equal(t, envList2, curEnv.Name)

		// Verify ListEnvs reflects the new current env
		envs, err := ListEnvs("")
		assert.NoError(t, err)
		for _, e := range envs {
			if e.Name == envList2 {
				assert.Equal(t, "*", e.Current)
			} else {
				assert.Equal(t, "", e.Current)
			}
		}
	})

	t.Run("get current env with legacy format", func(t *testing.T) {
		envPath, err := system.GetCurrentEnvPath()
		assert.NoError(t, err)
		err = os.WriteFile(envPath, []byte(envList1), 0644)
		assert.NoError(t, err)

		curEnv, err := GetCurrentEnv()
		assert.NoError(t, err)
		assert.Equal(t, envList1, curEnv.Name)
		assert.Equal(t, nsList1, curEnv.Namespace)

		// Check if the format was updated
		data, err := os.ReadFile(filepath.Clean(envPath))
		assert.NoError(t, err)
		var envMeta types.EnvMeta
		assert.NoError(t, json.Unmarshal(data, &envMeta))
		assert.Equal(t, envList1, envMeta.Name)
	})
}

func TestSetEnvLabels(t *testing.T) {
	ctx := context.Background()
	nsName := "set-labels-ns"
	envName := "test-labels"
	createTestNamespace(ctx, t, nsName)
	assert.NoError(t, CreateEnv(&types.EnvMeta{Name: envName, Namespace: nsName}))

	t.Run("set valid labels", func(t *testing.T) {
		envMeta := &types.EnvMeta{Name: envName, Labels: "foo=bar,hello=world"}
		err := SetEnvLabels(envMeta)
		assert.NoError(t, err)

		ns := &v1.Namespace{}
		err = k8sClient.Get(ctx, client.ObjectKey{Name: nsName}, ns)
		assert.NoError(t, err)
		assert.Equal(t, "bar", ns.Labels["foo"])
		assert.Equal(t, "world", ns.Labels["hello"])
	})

	t.Run("set labels on non-existent env", func(t *testing.T) {
		envMeta := &types.EnvMeta{Name: "non-existent-labels"}
		err := SetEnvLabels(envMeta)
		assert.Error(t, err)
	})

	t.Run("set invalid labels", func(t *testing.T) {
		envMeta := &types.EnvMeta{Name: envName, Labels: "invalid-label"}
		err := SetEnvLabels(envMeta)
		assert.Error(t, err)
	})
}

func TestDeleteEnv(t *testing.T) {
	ctx := context.Background()
	t.Run("delete env with application", func(t *testing.T) {
		nsName := "delete-env-ns-with-app"
		envName := "test-delete-with-app"
		createTestNamespace(ctx, t, nsName)
		assert.NoError(t, CreateEnv(&types.EnvMeta{Name: envName, Namespace: nsName}))

		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: nsName},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{{Name: "c1", Type: "worker"}},
			},
		}
		err := k8sClient.Create(ctx, app)
		assert.NoError(t, err)

		_, err = DeleteEnv(envName)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "you can't delete this environment")
	})

	t.Run("delete empty env", func(t *testing.T) {
		nsName := "delete-env-ns-empty"
		envName := "test-delete-empty"
		createTestNamespace(ctx, t, nsName)
		assert.NoError(t, CreateEnv(&types.EnvMeta{Name: envName, Namespace: nsName}))

		msg, err := DeleteEnv(envName)
		assert.NoError(t, err)
		assert.Equal(t, "env "+envName+" deleted", msg)

		ns := &v1.Namespace{}
		err = k8sClient.Get(ctx, client.ObjectKey{Name: nsName}, ns)
		assert.NoError(t, err)
		assert.Equal(t, "", ns.Labels[oam.LabelNamespaceOfEnvName])
		assert.Equal(t, "", ns.Labels[oam.LabelControlPlaneNamespaceUsage])
	})

	t.Run("delete non-existent env", func(t *testing.T) {
		_, err := DeleteEnv("non-existent-delete")
		assert.Error(t, err)
	})
}
