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

package multicluster

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	clusterv1alpha1 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ocmclusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestKubeClusterConfig_SetClusterName(t *testing.T) {
	testCases := []struct {
		name         string
		initialName  string
		newName      string
		expectedName string
	}{
		{
			name:         "Non-empty name",
			initialName:  "old",
			newName:      "new",
			expectedName: "new",
		},
		{
			name:         "Empty name",
			initialName:  "old",
			newName:      "",
			expectedName: "old",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &KubeClusterConfig{ClusterName: tc.initialName}
			out := cfg.SetClusterName(tc.newName)
			require.Equal(t, cfg, out)
			require.Equal(t, tc.expectedName, cfg.ClusterName)
		})
	}
}

func TestKubeClusterConfig_SetCreateNamespace(t *testing.T) {
	cfg := &KubeClusterConfig{}

	out := cfg.SetCreateNamespace("ns-1")
	require.Equal(t, cfg, out)
	require.Equal(t, "ns-1", cfg.CreateNamespace)

	out = cfg.SetCreateNamespace("")
	require.Equal(t, cfg, out)
	require.Equal(t, "", cfg.CreateNamespace)
}

func TestKubeClusterConfig_Validate(t *testing.T) {
	testCases := []struct {
		name        string
		clusterName string
		expectErr   bool
	}{
		{
			name:        "Empty name",
			clusterName: "",
			expectErr:   true,
		},
		{
			name:        "Local name",
			clusterName: ClusterLocalName,
			expectErr:   true,
		},
		{
			name:        "Valid name",
			clusterName: "prod",
			expectErr:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &KubeClusterConfig{ClusterName: tc.clusterName}
			err := cfg.Validate()
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = v1beta1.AddToScheme(s)
	_ = ocmclusterv1.AddToScheme(s)
	return s
}

// mockClient is a mock implementation of client.Client for testing.
// It allows injecting errors for different client operations.
type mockClient struct {
	client.Client
	listErr   error
	deleteErr error
	createErr error
	getErr    error
	updateErr error
}

func (m *mockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if m.getErr != nil {
		return m.getErr
	}
	return m.Client.Get(ctx, key, obj, opts...)
}

func (m *mockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if m.listErr != nil {
		if _, ok := list.(*v1beta1.ResourceTrackerList); ok {
			return m.listErr
		}
	}
	return m.Client.List(ctx, list, opts...)
}

func (m *mockClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if m.deleteErr != nil {
		if _, ok := obj.(*ocmclusterv1.ManagedCluster); ok {
			return m.deleteErr
		}
		if _, ok := obj.(*corev1.Secret); ok {
			return m.deleteErr
		}
	}
	return m.Client.Delete(ctx, obj, opts...)
}

func (m *mockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if m.createErr != nil {
		return m.createErr
	}
	return m.Client.Create(ctx, obj, opts...)
}

func (m *mockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	return m.Client.Update(ctx, obj, opts...)
}

func makeBaseClusterConfig(clusterName string) *KubeClusterConfig {
	return &KubeClusterConfig{
		FilePath:        "",
		ClusterName:     clusterName,
		CreateNamespace: "", // avoid PostRegistration side effects
		Config:          &clientcmdapi.Config{},
		Cluster: &clientcmdapi.Cluster{
			Server:                   "https://example:6443",
			CertificateAuthorityData: []byte("ca-bytes"),
			InsecureSkipTLSVerify:    false,
		},
		AuthInfo: &clientcmdapi.AuthInfo{},
	}
}

func TestRegisterByVelaSecret(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()
	oldNS := ClusterGatewaySecretNamespace
	ClusterGatewaySecretNamespace = "vela-system"
	t.Cleanup(func() { ClusterGatewaySecretNamespace = oldNS })

	testCases := []struct {
		name      string
		cfg       *KubeClusterConfig
		cli       client.Client
		expectErr bool
		verify    func(t *testing.T, cli client.Client, cfg *KubeClusterConfig)
	}{
		{
			name: "Token and endpoint",
			cfg: func() *KubeClusterConfig {
				cfg := makeBaseClusterConfig("c-token")
				cfg.AuthInfo.Token = "my-token"
				return cfg
			}(),
			cli: fake.NewClientBuilder().WithScheme(scheme).Build(),
			verify: func(t *testing.T, cli client.Client, cfg *KubeClusterConfig) {
				var sec corev1.Secret
				require.NoError(t, cli.Get(ctx, client.ObjectKey{Name: cfg.ClusterName, Namespace: ClusterGatewaySecretNamespace}, &sec))
				require.Equal(t, []byte("my-token"), sec.Data["token"])
				require.Equal(t, []byte("https://example:6443"), sec.Data["endpoint"])
				require.Equal(t, []byte("ca-bytes"), sec.Data["ca.crt"])
				require.Equal(t, string(clusterv1alpha1.CredentialTypeServiceAccountToken), sec.Labels[clustercommon.LabelKeyClusterCredentialType])
			},
		},
		{
			name: "Token no CA when insecure",
			cfg: func() *KubeClusterConfig {
				cfg := makeBaseClusterConfig("c-token-insecure")
				cfg.Cluster.InsecureSkipTLSVerify = true
				cfg.AuthInfo.Token = "tok"
				return cfg
			}(),
			cli: fake.NewClientBuilder().WithScheme(scheme).Build(),
			verify: func(t *testing.T, cli client.Client, cfg *KubeClusterConfig) {
				var sec corev1.Secret
				require.NoError(t, cli.Get(ctx, client.ObjectKey{Name: cfg.ClusterName, Namespace: ClusterGatewaySecretNamespace}, &sec))
				require.Nil(t, sec.Data["ca.crt"])
			},
		},
		{
			name: "Exec success",
			cfg: func() *KubeClusterConfig {
				dir := t.TempDir()
				script := filepath.Join(dir, "print-token.sh")
				require.NoError(t, os.WriteFile(script, []byte("#!/usr/bin/env bash\necho '{\"status\":{\"token\":\"exec-token\"}}'\n"), 0755))
				cfg := makeBaseClusterConfig("c-exec")
				cfg.AuthInfo.Exec = &clientcmdapi.ExecConfig{Command: script}
				return cfg
			}(),
			cli: fake.NewClientBuilder().WithScheme(scheme).Build(),
			verify: func(t *testing.T, cli client.Client, cfg *KubeClusterConfig) {
				var sec corev1.Secret
				require.NoError(t, cli.Get(ctx, client.ObjectKey{Name: cfg.ClusterName, Namespace: ClusterGatewaySecretNamespace}, &sec))
				require.Equal(t, []byte("exec-token"), sec.Data["token"])
				require.Equal(t, string(clusterv1alpha1.CredentialTypeServiceAccountToken), sec.Labels[clustercommon.LabelKeyClusterCredentialType])
			},
		},
		{
			name: "Exec failure",
			cfg: func() *KubeClusterConfig {
				dir := t.TempDir()
				cfg := makeBaseClusterConfig("c-exec-fail")
				cfg.AuthInfo.Exec = &clientcmdapi.ExecConfig{Command: filepath.Join(dir, "fail.sh")}
				require.NoError(t, os.WriteFile(cfg.AuthInfo.Exec.Command, []byte("#!/usr/bin/env bash\nexit 1\n"), 0755))
				return cfg
			}(),
			cli:       fake.NewClientBuilder().WithScheme(scheme).Build(),
			expectErr: true,
		},
		{
			name: "X509 and proxy",
			cfg: func() *KubeClusterConfig {
				cfg := makeBaseClusterConfig("c-x509")
				cfg.AuthInfo.ClientCertificateData = []byte("crt")
				cfg.AuthInfo.ClientKeyData = []byte("key")
				cfg.Cluster.ProxyURL = "http://proxy.example:8080"
				return cfg
			}(),
			cli: fake.NewClientBuilder().WithScheme(scheme).Build(),
			verify: func(t *testing.T, cli client.Client, cfg *KubeClusterConfig) {
				var sec corev1.Secret
				require.NoError(t, cli.Get(ctx, client.ObjectKey{Name: cfg.ClusterName, Namespace: ClusterGatewaySecretNamespace}, &sec))
				require.Equal(t, []byte("crt"), sec.Data["tls.crt"])
				require.Equal(t, []byte("key"), sec.Data["tls.key"])
				require.Equal(t, []byte("http://proxy.example:8080"), sec.Data["proxy-url"])
				require.Equal(t, string(clusterv1alpha1.CredentialTypeX509Certificate), sec.Labels[clustercommon.LabelKeyClusterCredentialType])
			},
		},
		{
			name: "Get error from createOrUpdate",
			cfg: func() *KubeClusterConfig {
				cfg := makeBaseClusterConfig("c-get-err")
				cfg.AuthInfo.Token = "tok"
				cfg.ClusterAlreadyExistCallback = func(string) bool { return true }
				return cfg
			}(),
			cli: func() client.Client {
				clusterName := "c-get-err"
				pre := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:            clusterName,
						Namespace:       ClusterGatewaySecretNamespace,
						Labels:          map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeServiceAccountToken)},
						ResourceVersion: "1",
					},
				}
				base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pre).Build()
				return &getErrorClient{Client: base, name: clusterName, namespace: ClusterGatewaySecretNamespace}
			}(),
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.RegisterByVelaSecret(ctx, tc.cli)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if tc.verify != nil {
				tc.verify(t, tc.cli, tc.cfg)
			}
		})
	}
}

func TestLoadKubeClusterConfigFromFile(t *testing.T) {
	testCases := []struct {
		name      string
		content   string
		expectErr bool
		verify    func(t *testing.T, cfg *KubeClusterConfig)
	}{
		{
			name: "Valid kubeconfig",
			content: `
apiVersion: v1
clusters:
- cluster:
    server: https://example.com
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
kind: Config
users:
- name: test-user
  user:
    token: test-token
`,
			verify: func(t *testing.T, cfg *KubeClusterConfig) {
				require.NotNil(t, cfg)
				require.Equal(t, "test-cluster", cfg.ClusterName)
				require.Equal(t, "https://example.com:443", cfg.Cluster.Server)
				require.Equal(t, "test-token", cfg.AuthInfo.Token)
			},
		},
		{
			name:      "File does not exist",
			content:   "",
			expectErr: true,
		},
		{
			name:      "Invalid kubeconfig",
			content:   "invalid-yaml",
			expectErr: true,
		},
		{
			name: "No current context",
			content: `
apiVersion: v1
clusters:
- cluster:
    server: https://example.com
  name: test-cluster
`,
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			if tc.content != "" {
				tmpfile, err := os.CreateTemp("", "kubeconfig")
				require.NoError(t, err)
				defer os.Remove(tmpfile.Name())
				_, err = tmpfile.Write([]byte(tc.content))
				require.NoError(t, err)
				err = tmpfile.Close()
				require.NoError(t, err)
				path = tmpfile.Name()
			} else {
				path = "/non-existent-file"
			}

			cfg, err := LoadKubeClusterConfigFromFile(path)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if tc.verify != nil {
				tc.verify(t, cfg)
			}
		})
	}
}

func TestDetachCluster(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()
	ClusterGatewaySecretNamespace = "vela-system"

	testCases := []struct {
		name        string
		clusterName string
		options     []DetachClusterOption
		cli         client.Client
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name: "removeClusterFromResourceTrackers returns error",
			cli: &mockClient{
				Client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
				listErr: errors.New("list error"),
			},
			clusterName: "any-cluster",
			wantErr:     true,
			wantErrMsg:  "list error",
		},
		{
			name:        "Detach local returns ErrReservedLocalClusterName",
			cli:         fake.NewClientBuilder().WithScheme(scheme).Build(),
			clusterName: ClusterLocalName,
			wantErr:     true,
			wantErrMsg:  ErrReservedLocalClusterName.Error(),
		},
		{
			name: "OCM Loading kubeconfig fails",
			cli: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ocm-load-cfg-fail",
					Namespace: ClusterGatewaySecretNamespace,
					Labels:    map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeOCMManagedCluster)},
				},
			}).Build(),
			clusterName: "ocm-load-cfg-fail",
			options:     []DetachClusterOption{DetachClusterManagedClusterKubeConfigPathOption("non-existent-path")},
			wantErr:     true,
		},
		{
			name: "OCM BuildConfig fails",
			cli: func() client.Client {
				tmpfile, err := os.CreateTemp("", "kubeconfig")
				require.NoError(t, err)
				defer os.Remove(tmpfile.Name())
				_, err = tmpfile.Write([]byte("invalid kubeconfig"))
				require.NoError(t, err)
				err = tmpfile.Close()
				require.NoError(t, err)
				return fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ocm-build-cfg-fail",
						Namespace: ClusterGatewaySecretNamespace,
						Labels:    map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeOCMManagedCluster)},
					},
				}).Build()
			}(),
			clusterName: "ocm-build-cfg-fail",
			options: func() []DetachClusterOption {
				tmpfile, err := os.CreateTemp("", "kubeconfig")
				require.NoError(t, err)
				t.Cleanup(func() { os.Remove(tmpfile.Name()) })
				_, err = tmpfile.Write([]byte("invalid kubeconfig"))
				require.NoError(t, err)
				err = tmpfile.Close()
				require.NoError(t, err)
				return []DetachClusterOption{DetachClusterManagedClusterKubeConfigPathOption(tmpfile.Name())}
			}(),
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := DetachCluster(ctx, tc.cli, tc.clusterName, tc.options...)
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrMsg != "" {
					require.Contains(t, err.Error(), tc.wantErrMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRenameCluster(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()
	ClusterGatewaySecretNamespace = "vela-system"

	testCases := []struct {
		name           string
		oldClusterName string
		newClusterName string
		cli            client.Client
		wantErr        bool
		wantErrMsg     string
		postCheck      func(t *testing.T, cli client.Client)
	}{
		{
			name:           "New name is local: returns ErrReservedLocalClusterName",
			oldClusterName: "old-cluster",
			newClusterName: ClusterLocalName,
			cli:            fake.NewClientBuilder().WithScheme(scheme).Build(),
			wantErr:        true,
			wantErrMsg:     ErrReservedLocalClusterName.Error(),
		},
		{
			name:           "getMutableClusterSecret error: wraps with 'is not mutable now'",
			oldClusterName: "non-existent-cluster",
			newClusterName: "new-cluster",
			cli:            fake.NewClientBuilder().WithScheme(scheme).Build(),
			wantErr:        true,
			wantErrMsg:     "is not mutable now",
		},
		{
			name:           "ensureClusterNotExists returns ErrClusterExists: error returned",
			oldClusterName: "old-cluster",
			newClusterName: "existing-cluster",
			cli: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "old-cluster",
						Namespace: ClusterGatewaySecretNamespace,
						Labels:    map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate)},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "existing-cluster",
						Namespace: ClusterGatewaySecretNamespace,
						Labels:    map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate)},
					},
					Data: map[string][]byte{"endpoint": []byte("https://example.com")},
				},
			).Build(),
			wantErr:    true,
			wantErrMsg: ErrClusterExists.Error(),
		},
		{
			name:           "Delete old secret fails: error",
			oldClusterName: "old-cluster",
			newClusterName: "new-cluster",
			cli: &mockClient{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "old-cluster",
						Namespace: ClusterGatewaySecretNamespace,
						Labels:    map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate)},
					},
				}).Build(),
				deleteErr: errors.New("delete failed"),
			},
			wantErr:    true,
			wantErrMsg: "delete failed",
		},
		{
			name:           "Create new secret fails: error",
			oldClusterName: "old-cluster",
			newClusterName: "new-cluster",
			cli: &mockClient{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "old-cluster",
						Namespace: ClusterGatewaySecretNamespace,
						Labels:    map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate)},
					},
				}).Build(),
				createErr: errors.New("create failed"),
			},
			wantErr:    true,
			wantErrMsg: "create failed",
		},
		{
			name:           "Success: Old deleted, new created with same labels/annotations",
			oldClusterName: "old-cluster",
			newClusterName: "new-cluster",
			cli: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "old-cluster",
					Namespace:   ClusterGatewaySecretNamespace,
					Labels:      map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate), "label-key": "label-value"},
					Annotations: map[string]string{"anno-key": "anno-value"},
				},
				Data: map[string][]byte{"key": []byte("value")},
			}).Build(),
			postCheck: func(t *testing.T, cli client.Client) {
				err := cli.Get(ctx, client.ObjectKey{Name: "old-cluster", Namespace: ClusterGatewaySecretNamespace}, &corev1.Secret{})
				require.True(t, apierrors.IsNotFound(err))
				newSecret := &corev1.Secret{}
				err = cli.Get(ctx, client.ObjectKey{Name: "new-cluster", Namespace: ClusterGatewaySecretNamespace}, newSecret)
				require.NoError(t, err)
				require.Equal(t, "new-cluster", newSecret.Name)
				require.Equal(t, map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate), "label-key": "label-value"}, newSecret.Labels)
				require.Equal(t, map[string]string{"anno-key": "anno-value"}, newSecret.Annotations)
				require.Equal(t, map[string][]byte{"key": []byte("value")}, newSecret.Data)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := RenameCluster(ctx, tc.cli, tc.oldClusterName, tc.newClusterName)
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrMsg != "" {
					require.Contains(t, err.Error(), tc.wantErrMsg)
				}
			} else {
				require.NoError(t, err)
			}
			if tc.postCheck != nil {
				tc.postCheck(t, tc.cli)
			}
		})
	}
}

// mock client to inject Get error on second secret fetch (createOrUpdate path)
type getErrorClient struct {
	client.Client
	name      string
	namespace string
	count     int
}

func (g *getErrorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if key.Name == g.name && key.Namespace == g.namespace {
		g.count++
		if g.count >= 2 { // first call used by existence check, second by createOrUpdate
			return errors.New("injected get error")
		}
	}
	return g.Client.Get(ctx, key, obj, opts...)
}

func TestEnsureClusterNotExists(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()

	testCases := []struct {
		name      string
		cli       client.Client
		cluster   string
		expectErr bool
	}{
		{
			name:      "Cluster does not exist",
			cli:       fake.NewClientBuilder().WithScheme(scheme).Build(),
			cluster:   "non-existent",
			expectErr: false,
		},
		{
			name: "Cluster exists",
			cli: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-cluster",
					Namespace: ClusterGatewaySecretNamespace,
					Labels:    map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate)},
				},
				Data: map[string][]byte{"endpoint": []byte("https://example.com")},
			}).Build(),
			cluster:   "existing-cluster",
			expectErr: true,
		},
		{
			name: "Client error",
			cli: &mockClient{
				Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
				getErr: errors.New("client error"),
			},
			cluster:   "any-cluster",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ensureClusterNotExists(ctx, tc.cli, tc.cluster)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestEnsureNamespaceExists(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()

	testCases := []struct {
		name      string
		cli       client.Client
		cluster   string
		namespace string
		expectErr bool
	}{
		{
			name: "Namespace already exists",
			cli: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "existing-ns"},
			}).Build(),
			cluster:   "any-cluster",
			namespace: "existing-ns",
			expectErr: false,
		},
		{
			name:      "Namespace does not exist",
			cli:       fake.NewClientBuilder().WithScheme(scheme).Build(),
			cluster:   "any-cluster",
			namespace: "new-ns",
			expectErr: false,
		},
		{
			name: "Client Get error",
			cli: &mockClient{
				Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
				getErr: errors.New("client error"),
			},
			cluster:   "any-cluster",
			namespace: "any-ns",
			expectErr: true,
		},
		{
			name: "Client Create error",
			cli: &mockClient{
				Client:    fake.NewClientBuilder().WithScheme(scheme).Build(),
				createErr: errors.New("client error"),
			},
			cluster:   "any-cluster",
			namespace: "new-ns",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ensureNamespaceExists(ctx, tc.cli, tc.cluster, tc.namespace)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetMutableClusterSecret(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()
	ClusterGatewaySecretNamespace = "vela-system"

	testCases := []struct {
		name      string
		cli       client.Client
		cluster   string
		expectErr bool
	}{
		{
			name:      "Secret does not exist",
			cli:       fake.NewClientBuilder().WithScheme(scheme).Build(),
			cluster:   "non-existent",
			expectErr: true,
		},
		{
			name: "Secret exists but no credential type label",
			cli: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-label",
					Namespace: ClusterGatewaySecretNamespace,
				},
			}).Build(),
			cluster:   "no-label",
			expectErr: true,
		},
		{
			name: "Secret exists with credential type label",
			cli: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "with-label",
					Namespace: ClusterGatewaySecretNamespace,
					Labels:    map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate)},
				},
			}).Build(),
			cluster:   "with-label",
			expectErr: false,
		},
		{
			name: "Client Get error",
			cli: &mockClient{
				Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
				getErr: errors.New("client error"),
			},
			cluster:   "any-cluster",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := getMutableClusterSecret(ctx, tc.cli, tc.cluster)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRemoveClusterFromResourceTrackers(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()

	testCases := []struct {
		name      string
		cli       client.Client
		cluster   string
		expectErr bool
		verify    func(t *testing.T, cli client.Client)
	}{
		{
			name:      "No resource trackers",
			cli:       fake.NewClientBuilder().WithScheme(scheme).Build(),
			cluster:   "any-cluster",
			expectErr: false,
		},
		{
			name: "Resource trackers exist, but none reference the cluster",
			cli: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&v1beta1.ResourceTracker{
				ObjectMeta: metav1.ObjectMeta{Name: "rt-1"},
				Spec: v1beta1.ResourceTrackerSpec{
					ManagedResources: []v1beta1.ManagedResource{
						{ClusterObjectReference: common.ClusterObjectReference{Cluster: "other-cluster"}},
					},
				},
			}).Build(),
			cluster:   "any-cluster",
			expectErr: false,
		},
		{
			name: "Resource trackers exist and some reference the cluster",
			cli: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&v1beta1.ResourceTracker{
				ObjectMeta: metav1.ObjectMeta{Name: "rt-1"},
				Spec: v1beta1.ResourceTrackerSpec{
					ManagedResources: []v1beta1.ManagedResource{
						{ClusterObjectReference: common.ClusterObjectReference{Cluster: "cluster-to-remove"}},
						{ClusterObjectReference: common.ClusterObjectReference{Cluster: "other-cluster"}},
					},
				},
			}).Build(),
			cluster: "cluster-to-remove",
			verify: func(t *testing.T, cli client.Client) {
				var rt v1beta1.ResourceTracker
				require.NoError(t, cli.Get(ctx, client.ObjectKey{Name: "rt-1"}, &rt))
				require.Len(t, rt.Spec.ManagedResources, 1)
				require.Equal(t, "other-cluster", rt.Spec.ManagedResources[0].Cluster)
			},
		},
		{
			name: "Client List error",
			cli: &mockClient{
				Client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
				listErr: errors.New("client error"),
			},
			cluster:   "any-cluster",
			expectErr: true,
		},
		{
			name: "Client Update error",
			cli: &mockClient{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&v1beta1.ResourceTracker{
					ObjectMeta: metav1.ObjectMeta{Name: "rt-1"},
					Spec: v1beta1.ResourceTrackerSpec{
						ManagedResources: []v1beta1.ManagedResource{
							{ClusterObjectReference: common.ClusterObjectReference{Cluster: "cluster-to-remove"}},
						},
					},
				}).Build(),
				updateErr: errors.New("client error"),
			},
			cluster:   "cluster-to-remove",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := removeClusterFromResourceTrackers(ctx, tc.cli, tc.cluster)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if tc.verify != nil {
				tc.verify(t, tc.cli)
			}
		})
	}
}

func TestGetTokenFromExec(t *testing.T) {
	testCases := []struct {
		name      string
		execCfg   *clientcmdapi.ExecConfig
		setup     func(t *testing.T, cfg *clientcmdapi.ExecConfig)
		expectErr bool
	}{
		{
			name:    "Valid exec config",
			execCfg: &clientcmdapi.ExecConfig{},
			setup: func(t *testing.T, cfg *clientcmdapi.ExecConfig) {
				dir := t.TempDir()
				script := filepath.Join(dir, "test.sh")
				require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho '{\"status\":{\"token\":\"test-token\"}}'"), 0755))
				cfg.Command = script
			},
		},
		{
			name:      "Exec command fails",
			execCfg:   &clientcmdapi.ExecConfig{Command: "/bin/false"},
			expectErr: true,
		},
		{
			name:    "Invalid JSON output",
			execCfg: &clientcmdapi.ExecConfig{},
			setup: func(t *testing.T, cfg *clientcmdapi.ExecConfig) {
				dir := t.TempDir()
				script := filepath.Join(dir, "test.sh")
				require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho 'invalid-json'"), 0755))
				cfg.Command = script
			},
			expectErr: true,
		},
		{
			name:    "No token in JSON output",
			execCfg: &clientcmdapi.ExecConfig{},
			setup: func(t *testing.T, cfg *clientcmdapi.ExecConfig) {
				dir := t.TempDir()
				script := filepath.Join(dir, "test.sh")
				require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho '{\"status\":{}}'"), 0755))
				cfg.Command = script
			},
			expectErr: true,
		},
		{
			name:      "Command with invalid characters",
			execCfg:   &clientcmdapi.ExecConfig{Command: "/bin/echo; ls"},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup(t, tc.execCfg)
			}
			_, err := getTokenFromExec(tc.execCfg)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAliasCluster(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme()
	ClusterGatewaySecretNamespace = "vela-system"

	// The secret that will be used in some test cases
	clusterSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: ClusterGatewaySecretNamespace,
			Labels:    map[string]string{clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate)},
		},
		Data: map[string][]byte{
			"endpoint": []byte("https://example.com"),
		},
	}

	testCases := []struct {
		name        string
		clusterName string
		aliasName   string
		cli         client.Client
		wantErr     error  // for specific error types
		wantErrMsg  string // for substring match
		postCheck   func(t *testing.T, cli client.Client)
	}{
		{
			name:        "Successfully alias cluster",
			clusterName: "test-cluster",
			aliasName:   "my-alias",
			cli:         fake.NewClientBuilder().WithScheme(scheme).WithObjects(clusterSecret.DeepCopy()).Build(),
			postCheck: func(t *testing.T, cli client.Client) {
				updatedSecret := &corev1.Secret{}
				err := cli.Get(ctx, client.ObjectKey{Name: "test-cluster", Namespace: ClusterGatewaySecretNamespace}, updatedSecret)
				require.NoError(t, err)
				annotations := updatedSecret.GetAnnotations()
				require.NotNil(t, annotations)
				require.Equal(t, "my-alias", annotations[clusterv1alpha1.AnnotationClusterAlias])
			},
		},
		{
			name:        "Local cluster returns error",
			clusterName: ClusterLocalName,
			aliasName:   "some-alias",
			cli:         fake.NewClientBuilder().WithScheme(scheme).Build(),
			wantErr:     ErrReservedLocalClusterName,
		},
		{
			name:        "Cluster not found error",
			clusterName: "non-existent-cluster",
			aliasName:   "my-alias",
			cli:         fake.NewClientBuilder().WithScheme(scheme).Build(),
			wantErrMsg:  "no such cluster",
		},
		{
			name:        "GetVirtualCluster fails",
			clusterName: "test-cluster",
			aliasName:   "my-alias",
			cli: &mockClient{
				Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
				getErr: errors.New("get error"),
			},
			wantErrMsg: "get error",
		},
		{
			name:        "Client update fails",
			clusterName: "test-cluster",
			aliasName:   "my-alias",
			cli: &mockClient{
				Client:    fake.NewClientBuilder().WithScheme(scheme).WithObjects(clusterSecret.DeepCopy()).Build(),
				updateErr: errors.New("update failed"),
			},
			wantErrMsg: "update failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := AliasCluster(ctx, tc.cli, tc.clusterName, tc.aliasName)

			if tc.wantErr != nil {
				require.Equal(t, tc.wantErr, err)
			} else if tc.wantErrMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErrMsg)
			} else {
				require.NoError(t, err)
			}

			if tc.postCheck != nil {
				tc.postCheck(t, tc.cli)
			}
		})
	}
}
