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

package cli

import (
	"path/filepath"
	"testing"

	"github.com/form3tech-oss/jwt-go"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// writeServiceAccountKubeConfig writes a kubeconfig whose only credential is the token of the
// serviceaccount prod/builder.
func writeServiceAccountKubeConfig(t *testing.T) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "system:serviceaccount:prod:builder",
	}).SignedString([]byte("secret"))
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, clientcmd.WriteToFile(clientcmdapi.Config{
		Clusters:       map[string]*clientcmdapi.Cluster{"c": {Server: "https://example"}},
		Contexts:       map[string]*clientcmdapi.Context{"ctx": {Cluster: "c", AuthInfo: "ai"}},
		CurrentContext: "ctx",
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{"ai": {Token: token}},
	}, path))
	return path
}

// A serviceaccount read from --kubeconfig keeps the namespace of its token. The command line
// namespace only applies to an explicit --serviceaccount, and must not replace it.
func TestAuthCompleteKeepsKubeConfigServiceAccountNamespace(t *testing.T) {
	kubeconfig := writeServiceAccountKubeConfig(t)

	t.Run("grant-privileges", func(t *testing.T) {
		opt := &GrantPrivilegesOptions{KubeConfig: kubeconfig}
		opt.Complete(nil, nil)
		require.Equal(t, "builder", opt.Identity.ServiceAccount)
		require.Equal(t, "prod", opt.Identity.ServiceAccountNamespace)
	})

	t.Run("list-privileges", func(t *testing.T) {
		// list-privileges also reads the clusters from the command's flags.
		cmd := &cobra.Command{}
		cmd.Flags().StringSlice("cluster", nil, "")
		opt := &ListPrivilegesOptions{KubeConfig: kubeconfig}
		opt.Complete(nil, cmd)
		require.Equal(t, "builder", opt.Identity.ServiceAccount)
		require.Equal(t, "prod", opt.Identity.ServiceAccountNamespace)
	})
}
