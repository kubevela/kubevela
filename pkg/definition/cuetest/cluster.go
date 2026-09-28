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

package cuetest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/velaconfig"
	velaconfigreader "github.com/oam-dev/kubevela/pkg/cue/cuex/providers/velaconfig/reader"
	"github.com/oam-dev/kubevela/pkg/registry"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

// ClusterOptions configure the API server step and source cases run against.
type ClusterOptions struct {
	// Assets is the directory holding etcd and kube-apiserver; when empty,
	// KUBEBUILDER_ASSETS is used, as envtest does.
	Assets string
	// CRDs are files or directories of CRDs to install.
	CRDs []string
}

// TestCluster is a local API server (envtest) that steps and sources run
// against: real, but with no controllers, so nothing reconciles.
type TestCluster struct {
	Client client.Client
	Config *rest.Config

	env *envtest.Environment
	mu  sync.Mutex
	seq int
}

// StartTestCluster starts an API server with the given CRDs installed.
func StartTestCluster(opts ClusterOptions) (*TestCluster, error) {
	env := &envtest.Environment{
		CRDDirectoryPaths:     opts.CRDs,
		ErrorIfCRDPathMissing: true,
	}
	// envtest prefers KUBEBUILDER_ASSETS to any directory it is given, so an
	// explicit one is set there for the start.
	if opts.Assets != "" {
		saved, had := os.LookupEnv("KUBEBUILDER_ASSETS")
		_ = os.Setenv("KUBEBUILDER_ASSETS", opts.Assets)
		defer func() {
			if had {
				_ = os.Setenv("KUBEBUILDER_ASSETS", saved)
			} else {
				_ = os.Unsetenv("KUBEBUILDER_ASSETS")
			}
		}()
	}
	cfg, err := env.Start()
	if err != nil {
		return nil, fmt.Errorf("step and source tests run against a local API server (envtest), which did not start: %w; "+
			"install its binaries with `go run sigs.k8s.io/controller-runtime/tools/setup-envtest@latest use -p path` "+
			"and point KUBEBUILDER_ASSETS (or --envtest-assets) at them", err)
	}
	cli, err := client.New(cfg, client.Options{Scheme: common.Scheme})
	if err != nil {
		_ = env.Stop()
		return nil, err
	}
	return &TestCluster{Client: cli, Config: cfg, env: env}, nil
}

// Stop shuts the API server down.
func (c *TestCluster) Stop() error {
	return c.env.Stop()
}

// NewNamespace creates a namespace no case has used. envtest runs no
// namespace controller, so namespaces cannot be deleted cleanly; a fresh one
// per case is what keeps cases apart.
func (c *TestCluster) NewNamespace() (string, error) {
	c.mu.Lock()
	c.seq++
	name := fmt.Sprintf("cuetest-%d", c.seq)
	c.mu.Unlock()
	return name, c.ensureNamespace(name)
}

func (c *TestCluster) ensureNamespace(name string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := c.Client.Create(context.Background(), ns); err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("creating namespace %s: %w", name, err)
	}
	return nil
}

// Seed creates objects in namespace ns, unless they name their own, which is
// created if missing. Any status they carry is set as a controller would have,
// through the status subresource where the kind has one: nothing here
// reconciles, so nothing becomes ready by itself.
func (c *TestCluster) Seed(ns string, objs []map[string]any) error {
	return c.seed(ns, objs, nil)
}

// seed is Seed, adding each object it creates to created.
func (c *TestCluster) seed(ns string, objs []map[string]any, created *createdObjects) error {
	ctx := context.Background()
	for i, o := range objs {
		obj := &unstructured.Unstructured{Object: deepCopy(o)}
		// An unknown kind is left for Create to report.
		if namespaced, err := c.Client.IsObjectNamespaced(obj); err == nil && namespaced {
			if obj.GetNamespace() == "" {
				obj.SetNamespace(ns)
			}
			if err := c.ensureNamespace(obj.GetNamespace()); err != nil {
				return fmt.Errorf("resources[%d]: %w", i, err)
			}
		}
		status, hasStatus := obj.Object["status"]
		if err := c.Client.Create(ctx, obj); err != nil {
			return fmt.Errorf("resources[%d]: creating %s %s: %w", i, obj.GetKind(), obj.GetName(), err)
		}
		created.add(obj)
		// A kind with a status subresource drops status on create, so it is
		// set through that; one without stored it with the rest.
		if !hasStatus || equality.Semantic.DeepEqual(obj.Object["status"], status) {
			continue
		}
		obj.Object["status"] = status
		if err := c.Client.Status().Update(ctx, obj); err != nil {
			return fmt.Errorf("resources[%d]: setting the status of %s %s: %w", i, obj.GetKind(), obj.GetName(), err)
		}
	}
	return nil
}

// runtimeContext is ctx carrying what the controller gives a step's
// providers: this cluster's client and config, and a Config factory on it.
// The providers otherwise read the process's kubeconfig, and exit without
// one.
func (c *TestCluster) runtimeContext(ctx context.Context) context.Context {
	singleton.KubeClient.Set(c.Client)
	singleton.KubeConfig.Set(c.Config)
	// The controller registers how vela/velaconfig reads a Config at startup.
	if _, ok := registry.Get[velaconfig.Reader](); !ok {
		registry.RegisterAs[velaconfig.Reader](velaconfigreader.Config{Client: c.Client})
	}
	return oamprovidertypes.WithRuntimeParams(ctx, oamprovidertypes.RuntimeParams{
		KubeClient:    c.Client,
		KubeConfig:    c.Config,
		ConfigFactory: config.NewConfigFactory(c.Client),
	})
}

// installCRDs installs the CRDs at paths, every time: a hook, step or
// cleanup may have deleted one since it was last installed.
func (c *TestCluster) installCRDs(paths []string) error {
	_, err := envtest.InstallCRDs(c.Config, envtest.CRDInstallOptions{Paths: paths, ErrorIfPathMissing: true})
	return err
}

// findKubeVelaCRDs looks upward from dir for the vela-core chart's CRDs.
func findKubeVelaCRDs(dir string) (string, bool) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		crds := filepath.Join(dir, "charts", "vela-core", "crds")
		if info, err := os.Stat(crds); err == nil && info.IsDir() {
			return crds, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func deepCopy(m map[string]any) map[string]any {
	return (&unstructured.Unstructured{Object: m}).DeepCopy().Object
}

func isAlreadyExists(err error) bool {
	return apierrors.IsAlreadyExists(err)
}

// ExecClusterOptions configure the cluster step and source cases share.
// Set them before the first case runs. Without CRDs, the vela-core chart's
// are installed, found by looking upward from the first test file.
var ExecClusterOptions ClusterOptions

var execClusterState struct {
	sync.Mutex
	cluster *TestCluster
	err     error
	started bool
	// restore puts back the process-wide state the cluster's cases replace.
	restore func()
}

// execCluster is the cluster step and source cases share, started on first
// use: an API server costs seconds to start, so one serves a whole run.
func execCluster(near string) (*TestCluster, error) {
	s := &execClusterState
	s.Lock()
	defer s.Unlock()
	if !s.started {
		s.started = true
		s.restore = saveProcessState()
		opts := ExecClusterOptions
		if len(opts.CRDs) == 0 {
			if dir, ok := findKubeVelaCRDs(near); ok {
				opts.CRDs = []string{dir}
			}
		}
		s.cluster, s.err = StartTestCluster(opts)
	}
	return s.cluster, s.err
}

// StopExecCluster stops the cluster step and source cases ran against, if
// one started, since its processes outlive this one otherwise, puts back the
// process-wide client, config and provider registry as they were before it
// started, and forgets any failure to start one, so the next run tries again.
func StopExecCluster() error {
	s := &execClusterState
	s.Lock()
	defer s.Unlock()
	var err error
	if s.cluster != nil {
		err = s.cluster.Stop()
	}
	if s.restore != nil {
		s.restore()
	}
	s.cluster, s.err, s.started, s.restore = nil, nil, false, nil
	return err
}

// saveProcessState captures the process-wide state runtimeContext replaces,
// and returns a function putting it back.
func saveProcessState() func() {
	restoreClient := saveSingleton(singleton.KubeClient)
	restoreConfig := saveSingleton(singleton.KubeConfig)
	providers := registry.Snapshot()
	return func() {
		restoreClient()
		restoreConfig()
		registry.Restore(providers)
	}
}

// saveSingleton copies s whole, loader included, and returns a function
// putting the copy back: a Singleton's API cannot unset a value, and reading
// one never set would load it from the process's kubeconfig.
func saveSingleton[T any](s *singleton.Singleton[T]) func() {
	v := reflect.ValueOf(s).Elem()
	saved := reflect.New(v.Type()).Elem()
	saved.Set(v)
	return func() { v.Set(saved) }
}
