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

package util

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/oam-dev/kubevela/version"
)

func TestGenerateLeaderElectionID(t *testing.T) {
	orig := version.VelaVersion
	defer func() { version.VelaVersion = orig }()

	testCases := []struct {
		name      string
		ver       string
		versioned bool
		want      string
	}{
		{name: "versioned", ver: "v10.13.0", versioned: true, want: "kubevela-v10-13-0"},
		{name: "unversioned", ver: "v10.13.0", versioned: false, want: "kubevela"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			version.VelaVersion = tc.ver
			got := GenerateLeaderElectionID("kubevela", tc.versioned)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestComputeDiscoverCacheDir(t *testing.T) {
	t.Parallel()
	parent := "/parent"

	testCases := []struct {
		name string
		host string
		want string
	}{
		{name: "https scheme removed", host: "https://example.com:6443", want: "example.com_6443"},
		{name: "http scheme removed", host: "http://example.com:6443", want: "example.com_6443"},
		{name: "no scheme", host: "k8s.example.io:443", want: "k8s.example.io_443"},
		{name: "sanitize special", host: "exa_mple.com:6443?x=y#frag", want: "exa_mple.com_6443_x_y_frag"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := computeDiscoverCacheDir(parent, tc.host)
			want := filepath.Join(parent, tc.want)
			require.Equal(t, want, got)
		})
	}
}

func TestRestConfigGetter(t *testing.T) {
	t.Parallel()

	t.Run("ToRESTConfig returns same pointer", func(t *testing.T) {
		t.Parallel()
		cfg := &rest.Config{Host: "https://cluster.local"}
		r := &restConfigGetter{config: cfg, namespace: "ns"}
		got, err := r.ToRESTConfig()
		require.NoError(t, err)
		require.Same(t, cfg, got)
	})

	t.Run("NewRestConfigGetterByConfig sets fields", func(t *testing.T) {
		t.Parallel()
		cfg := &rest.Config{Host: "https://cluster.local"}
		g := NewRestConfigGetterByConfig(cfg, "my-ns")
		r, ok := g.(*restConfigGetter)
		require.True(t, ok, "unexpected underlying type for RESTClientGetter")
		require.Equal(t, "my-ns", r.namespace)
		require.Same(t, cfg, r.config)
	})
}

func TestRestConfigGetter_ToRawKubeConfigLoader(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		config     *rest.Config
		namespace  string
		assertFunc func(*testing.T, clientcmd.ClientConfig)
	}{
		{
			name: "simple config",
			config: &rest.Config{
				Host: "https://my-cluster.local",
			},
			namespace: "my-ns",
			assertFunc: func(t *testing.T, clientConfig clientcmd.ClientConfig) {
				ns, _, err := clientConfig.Namespace()
				require.NoError(t, err)
				require.Equal(t, "my-ns", ns)
			},
		},
		{
			name: "full config",
			config: &rest.Config{
				Host:        "https://my-cluster.local:6443",
				Username:    "my-user",
				Password:    "my-pass",
				BearerToken: "my-token",
				TLSClientConfig: rest.TLSClientConfig{
					CertFile: "/path/to/cert",
					KeyFile:  "/path/to/key",
					CAFile:   "/path/to/ca",
					Insecure: true,
				},
				Timeout: 30 * time.Second,
				Impersonate: rest.ImpersonationConfig{
					UserName: "impersonate-user",
					Groups:   []string{"group1", "group2"},
					Extra:    map[string][]string{"extra": {"val"}},
				},
			},
			namespace: "full-ns",
			assertFunc: func(t *testing.T, clientConfig clientcmd.ClientConfig) {
				ns, _, err := clientConfig.Namespace()
				require.NoError(t, err)
				require.Equal(t, "full-ns", ns)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			getter := NewRestConfigGetterByConfig(tc.config, tc.namespace)
			clientConfig := getter.ToRawKubeConfigLoader()
			require.NotNil(t, clientConfig)
			tc.assertFunc(t, clientConfig)
		})
	}
}

func TestRestConfigGetter_DiscoveryAndMapper(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	originalDefaultCacheDir := defaultCacheDir
	defaultCacheDir = filepath.Join(tmpHome, ".kube", "http-cache")
	defer func() { defaultCacheDir = originalDefaultCacheDir }()

	cfg := &rest.Config{Host: "http://127.0.0.1:1"}
	getter := NewRestConfigGetterByConfig(cfg, "default")

	t.Run("ToDiscoveryClient", func(t *testing.T) {
		discoveryClient, err := getter.ToDiscoveryClient()
		require.NoError(t, err)
		require.NotNil(t, discoveryClient)
	})

	t.Run("ToRESTMapper", func(t *testing.T) {
		mapper, err := getter.ToRESTMapper()
		require.NoError(t, err)
		require.NotNil(t, mapper)
	})
}
