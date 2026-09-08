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

package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	velatypes "github.com/oam-dev/kubevela/apis/types"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// parseModuleRegistry builds a registry from positional args and flags, the way
// the add command does, without needing a cluster.
func parseModuleRegistry(args []string, flags ...string) (*pkgaddon.Registry, error) {
	cmd := &cobra.Command{}
	addModuleRegistryFlags(cmd)
	if err := cmd.Flags().Parse(flags); err != nil {
		return nil, err
	}
	return moduleRegistryFromArgs(cmd, args)
}

// moduleArgs returns common.Args backed by a fake client, so the command helpers
// can run without a kubeconfig.
func moduleArgs(t *testing.T) common.Args {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	c := common.Args{}
	c.SetClient(fake.NewClientBuilder().WithScheme(scheme).Build())
	return c
}

func TestModuleRegistryFromArgs(t *testing.T) {
	t.Run("git registry defaults the path to module", func(t *testing.T) {
		reg, err := parseModuleRegistry(
			[]string{"catalog", "https://github.com/kubevela/catalog"}, "--type=git")
		require.NoError(t, err)
		require.NotNil(t, reg.Git)
		assert.Nil(t, reg.OCIChartSource())
		assert.Equal(t, "catalog", reg.Name)
		assert.Equal(t, "https://github.com/kubevela/catalog", reg.Git.URL)
		assert.Equal(t, pkgmodule.DefaultGitPath, reg.Git.Path)
	})

	t.Run("path flag overrides the default", func(t *testing.T) {
		reg, err := parseModuleRegistry(
			[]string{"catalog", "https://github.com/kubevela/catalog"}, "--type=git", "--path=.")
		require.NoError(t, err)
		assert.Equal(t, ".", reg.Git.Path)
	})

	t.Run("git token is carried on the source", func(t *testing.T) {
		reg, err := parseModuleRegistry(
			[]string{"private", "https://github.com/org/private"}, "--type=git", "--gitToken=t0ken")
		require.NoError(t, err)
		assert.Equal(t, "t0ken", reg.Git.Token)
	})

	t.Run("oci registry carries username and password", func(t *testing.T) {
		reg, err := parseModuleRegistry(
			[]string{"ghcr", "oci://ghcr.io/org/modules"},
			"--type=oci", "--username=robot", "--password=secret")
		require.NoError(t, err)
		oci := reg.OCIChartSource()
		require.NotNil(t, oci)
		assert.Nil(t, reg.Git)
		assert.Equal(t, "oci://ghcr.io/org/modules", oci.URL)
		assert.Equal(t, "robot", oci.Username)
		assert.Equal(t, "secret", oci.Token)
	})

	t.Run("type is inferred from the URL", func(t *testing.T) {
		cases := map[string]string{
			"https://github.com/org/catalog":  moduleGitType,
			"http://git.internal/org/catalog": moduleGitType,
			"git@github.com:org/catalog.git":  moduleGitType,
			"oci://ghcr.io/org/modules":       moduleOCIType,
		}
		for url, wantType := range cases {
			reg, err := parseModuleRegistry([]string{"catalog", url})
			require.NoError(t, err, url)
			if wantType == moduleGitType {
				assert.NotNil(t, reg.Git, url)
			} else {
				assert.NotNil(t, reg.OCIChartSource(), url)
			}
		}
	})

	t.Run("ambiguous URL asks for the type", func(t *testing.T) {
		_, err := parseModuleRegistry([]string{"catalog", "github.com/guidewire-oss/catalog"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--type")
	})

	t.Run("invalid name is rejected", func(t *testing.T) {
		_, err := parseModuleRegistry(
			[]string{"MyCatalog", "https://github.com/org/catalog"}, "--type=git")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MyCatalog")
		assert.Contains(t, err.Error(), "DNS subdomain")
	})

	t.Run("unknown type is rejected", func(t *testing.T) {
		_, err := parseModuleRegistry(
			[]string{"catalog", "https://github.com/org/catalog"}, "--type=helm")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "helm")
	})

	t.Run("wrong argument count is rejected", func(t *testing.T) {
		_, err := parseModuleRegistry([]string{"catalog"}, "--type=git")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name and URL")
	})

	t.Run("empty name is rejected", func(t *testing.T) {
		_, err := parseModuleRegistry(
			[]string{"", "https://github.com/org/catalog"}, "--type=git")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DNS subdomain")
	})

	t.Run("a name too long for its token secret is rejected", func(t *testing.T) {
		// module-registry- (16 chars) + a 250-char name = 266, over the
		// Secret's 253-char DNS subdomain limit, even though the name alone
		// is short enough to pass on its own.
		name := strings.Repeat("a", 250)
		_, err := parseModuleRegistry(
			[]string{name, "https://github.com/org/catalog"}, "--type=git")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too long")
		assert.Contains(t, err.Error(), pkgmodule.ModuleRegistrySecretPrefix+name)
	})

	t.Run("oci username without password is rejected", func(t *testing.T) {
		_, err := parseModuleRegistry(
			[]string{"ghcr", "oci://ghcr.io/org/modules"}, "--type=oci", "--username=robot")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "username and password must be supplied together")
	})

	t.Run("oci password without username is rejected", func(t *testing.T) {
		_, err := parseModuleRegistry(
			[]string{"ghcr", "oci://ghcr.io/org/modules"}, "--type=oci", "--password=secret")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "username and password must be supplied together")
	})
}

func TestRequireModuleRegistryName(t *testing.T) {
	t.Run("a single name is accepted", func(t *testing.T) {
		name, err := requireModuleRegistryName([]string{"catalog"})
		require.NoError(t, err)
		assert.Equal(t, "catalog", name)
	})

	t.Run("wrong argument count is rejected", func(t *testing.T) {
		_, err := requireModuleRegistryName([]string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must specify the registry name")
	})

	t.Run("an empty name is rejected", func(t *testing.T) {
		_, err := requireModuleRegistryName([]string{""})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not be empty")
	})

	t.Run("a whitespace-only name is rejected", func(t *testing.T) {
		// The trigger this guards against is ordinary scripting, e.g.
		// `vela module registry delete "$REG"` with REG unset, which leaves
		// args[0] empty (or, via some shells, whitespace).
		_, err := requireModuleRegistryName([]string{"   "})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not be empty")
	})
}

func TestAddModuleRegistry(t *testing.T) {
	ctx := context.Background()
	reg := pkgaddon.Registry{
		Name: "catalog",
		Git:  &pkgaddon.GitAddonSource{URL: "https://github.com/kubevela/catalog", Path: "module"},
	}

	t.Run("adds a new registry", func(t *testing.T) {
		c := moduleArgs(t)
		var out bytes.Buffer
		require.NoError(t, addModuleRegistry(ctx, c, reg, false, &out))
		assert.Contains(t, out.String(), "catalog")

		k8sClient, err := c.GetClient()
		require.NoError(t, err)
		got, err := pkgmodule.NewStore(k8sClient).GetRegistry(ctx, "catalog")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/kubevela/catalog", got.Git.URL)
	})

	t.Run("duplicate without force is rejected", func(t *testing.T) {
		c := moduleArgs(t)
		var out bytes.Buffer
		require.NoError(t, addModuleRegistry(ctx, c, reg, false, &out))

		err := addModuleRegistry(ctx, c, reg, false, &out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
		assert.Contains(t, err.Error(), "--force")
	})

	t.Run("duplicate with force updates in place", func(t *testing.T) {
		c := moduleArgs(t)
		var out bytes.Buffer
		require.NoError(t, addModuleRegistry(ctx, c, reg, false, &out))

		updated := pkgaddon.Registry{
			Name: "catalog",
			Git:  &pkgaddon.GitAddonSource{URL: "https://github.com/org/fork", Path: "module"},
		}
		require.NoError(t, addModuleRegistry(ctx, c, updated, true, &out))

		k8sClient, err := c.GetClient()
		require.NoError(t, err)
		got, err := pkgmodule.NewStore(k8sClient).GetRegistry(ctx, "catalog")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/org/fork", got.Git.URL)
	})

	t.Run("force overwrite without a new token keeps the stored credential", func(t *testing.T) {
		c := moduleArgs(t)
		var out bytes.Buffer
		withToken := pkgaddon.Registry{
			Name: "catalog",
			Git:  &pkgaddon.GitAddonSource{URL: "https://github.com/kubevela/catalog", Path: "module", Token: "t0ken"},
		}
		require.NoError(t, addModuleRegistry(ctx, c, withToken, false, &out))

		k8sClient, err := c.GetClient()
		require.NoError(t, err)
		secretName := pkgmodule.ModuleRegistrySecretPrefix + "catalog"
		var secretBefore corev1.Secret
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Namespace: velatypes.DefaultKubeVelaNS, Name: secretName}, &secretBefore),
			"the token must have been migrated to a secret")

		withoutToken := pkgaddon.Registry{
			Name: "catalog",
			Git:  &pkgaddon.GitAddonSource{URL: "https://github.com/org/fork", Path: "module"},
		}
		require.NoError(t, addModuleRegistry(ctx, c, withoutToken, true, &out))

		after, err := pkgmodule.NewStore(k8sClient).GetRegistry(ctx, "catalog")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/org/fork", after.Git.URL)
		assert.Equal(t, "t0ken", after.Git.Token,
			"the stored token must survive a force update with no new token")

		var secretAfter corev1.Secret
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Namespace: velatypes.DefaultKubeVelaNS, Name: secretName}, &secretAfter),
			"the secret must not be orphaned or deleted")
		assert.Equal(t, "t0ken", string(secretAfter.Data["token"]))
	})
}

func TestUpdateModuleRegistry(t *testing.T) {
	ctx := context.Background()

	t.Run("updating a nonexistent registry errors", func(t *testing.T) {
		var out bytes.Buffer
		err := updateModuleRegistry(ctx, moduleArgs(t), pkgaddon.Registry{
			Name: "catalog",
			Git:  &pkgaddon.GitAddonSource{URL: "https://github.com/kubevela/catalog", Path: "module"},
		}, &out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "catalog")
	})

	t.Run("updating an existing registry changes it", func(t *testing.T) {
		c := moduleArgs(t)
		var discard bytes.Buffer
		require.NoError(t, addModuleRegistry(ctx, c, pkgaddon.Registry{
			Name: "catalog",
			Git:  &pkgaddon.GitAddonSource{URL: "https://github.com/kubevela/catalog", Path: "module"},
		}, false, &discard))

		var out bytes.Buffer
		require.NoError(t, updateModuleRegistry(ctx, c, pkgaddon.Registry{
			Name: "catalog",
			Git:  &pkgaddon.GitAddonSource{URL: "https://github.com/org/fork", Path: "module"},
		}, &out))
		assert.Contains(t, out.String(), "catalog")

		k8sClient, err := c.GetClient()
		require.NoError(t, err)
		got, err := pkgmodule.NewStore(k8sClient).GetRegistry(ctx, "catalog")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/org/fork", got.Git.URL)
	})

	t.Run("updating without a new token keeps the stored credential", func(t *testing.T) {
		c := moduleArgs(t)
		var discard bytes.Buffer
		require.NoError(t, addModuleRegistry(ctx, c, pkgaddon.Registry{
			Name: "catalog",
			Git:  &pkgaddon.GitAddonSource{URL: "https://github.com/kubevela/catalog", Path: "module", Token: "t0ken"},
		}, false, &discard))

		k8sClient, err := c.GetClient()
		require.NoError(t, err)
		secretName := pkgmodule.ModuleRegistrySecretPrefix + "catalog"
		var secretBefore corev1.Secret
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Namespace: velatypes.DefaultKubeVelaNS, Name: secretName}, &secretBefore),
			"the token must have been migrated to a secret")

		require.NoError(t, updateModuleRegistry(ctx, c, pkgaddon.Registry{
			Name: "catalog",
			Git:  &pkgaddon.GitAddonSource{URL: "https://github.com/org/fork", Path: "module"},
		}, &discard))

		after, err := pkgmodule.NewStore(k8sClient).GetRegistry(ctx, "catalog")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/org/fork", after.Git.URL)
		assert.Equal(t, "t0ken", after.Git.Token,
			"the stored token must survive an update with no new token")

		var secretAfter corev1.Secret
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Namespace: velatypes.DefaultKubeVelaNS, Name: secretName}, &secretAfter),
			"the secret must not be orphaned or deleted")
		assert.Equal(t, "t0ken", string(secretAfter.Data["token"]))
	})
}

func TestListModuleRegistry(t *testing.T) {
	ctx := context.Background()

	t.Run("empty store prints headers only", func(t *testing.T) {
		var out bytes.Buffer
		require.NoError(t, listModuleRegistry(ctx, moduleArgs(t), &out))

		printed := out.String()
		for _, header := range []string{"NAME", "TYPE", "URL", "PATH"} {
			assert.Contains(t, printed, header)
		}

		var rows []string
		for _, line := range strings.Split(printed, "\n") {
			if strings.TrimSpace(line) != "" {
				rows = append(rows, line)
			}
		}
		assert.Len(t, rows, 1, "expected only the header row, got: %q", printed)
	})

	t.Run("prints git and oci rows sorted by name", func(t *testing.T) {
		c := moduleArgs(t)
		var discard bytes.Buffer
		require.NoError(t, addModuleRegistry(ctx, c, pkgaddon.Registry{
			Name: "zzz-oci",
			Helm: &pkgaddon.HelmSource{URL: "oci://ghcr.io/org/modules"},
		}, false, &discard))
		require.NoError(t, addModuleRegistry(ctx, c, pkgaddon.Registry{
			Name: "catalog",
			Git:  &pkgaddon.GitAddonSource{URL: "https://github.com/kubevela/catalog", Path: "module"},
		}, false, &discard))

		var out bytes.Buffer
		require.NoError(t, listModuleRegistry(ctx, c, &out))
		printed := out.String()
		assert.Contains(t, printed, "catalog")
		assert.Contains(t, printed, "git")
		assert.Contains(t, printed, "module")
		assert.Contains(t, printed, "zzz-oci")
		assert.Contains(t, printed, "oci")
		assert.Less(t, strings.Index(printed, "catalog"), strings.Index(printed, "zzz-oci"),
			"rows must be sorted by name")
	})

	t.Run("shows an unsupported entry with its type named", func(t *testing.T) {
		// The module ConfigMap shares its format with the addon one, so a
		// helm entry can be present -- hand-edited, or written by
		// `vela addon registry` if pointed at this ConfigMap. An operator
		// has to be able to see it in order to remove it.
		c := moduleArgs(t)
		var discard bytes.Buffer
		require.NoError(t, addModuleRegistry(ctx, c, pkgaddon.Registry{
			Name: "legacy",
			Helm: &pkgaddon.HelmSource{URL: "https://charts.example.com/legacy"},
		}, false, &discard))

		var out bytes.Buffer
		require.NoError(t, listModuleRegistry(ctx, c, &out))
		printed := out.String()
		assert.Contains(t, printed, "legacy")
		assert.Contains(t, printed, "helm")
		assert.Contains(t, printed, "https://charts.example.com/legacy")
	})
}

func TestGetModuleRegistry(t *testing.T) {
	ctx := context.Background()

	t.Run("prints the named registry", func(t *testing.T) {
		c := moduleArgs(t)
		var discard bytes.Buffer
		require.NoError(t, addModuleRegistry(ctx, c, pkgaddon.Registry{
			Name: "catalog",
			Git:  &pkgaddon.GitAddonSource{URL: "https://github.com/kubevela/catalog", Path: "module"},
		}, false, &discard))

		var out bytes.Buffer
		require.NoError(t, getModuleRegistry(ctx, c, "catalog", &out))
		assert.Contains(t, out.String(), "catalog")
		assert.Contains(t, out.String(), "https://github.com/kubevela/catalog")
	})

	t.Run("unknown name errors", func(t *testing.T) {
		var out bytes.Buffer
		err := getModuleRegistry(ctx, moduleArgs(t), "nope", &out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nope")
	})

	t.Run("an unsupported entry errors clearly", func(t *testing.T) {
		c := moduleArgs(t)
		var discard bytes.Buffer
		require.NoError(t, addModuleRegistry(ctx, c, pkgaddon.Registry{
			Name: "legacy",
			Helm: &pkgaddon.HelmSource{URL: "https://charts.example.com/legacy"},
		}, false, &discard))

		var out bytes.Buffer
		err := getModuleRegistry(ctx, c, "legacy", &out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "legacy")
		assert.Contains(t, err.Error(), "helm")
	})
}

func TestDeleteModuleRegistry(t *testing.T) {
	ctx := context.Background()

	t.Run("removes only the named registry", func(t *testing.T) {
		c := moduleArgs(t)
		var discard bytes.Buffer
		require.NoError(t, addModuleRegistry(ctx, c, pkgaddon.Registry{
			Name: "catalog",
			Git:  &pkgaddon.GitAddonSource{URL: "https://github.com/kubevela/catalog", Path: "module"},
		}, false, &discard))
		require.NoError(t, addModuleRegistry(ctx, c, pkgaddon.Registry{
			Name: "mine",
			Git:  &pkgaddon.GitAddonSource{URL: "https://github.com/org/mine", Path: "module"},
		}, false, &discard))

		var out bytes.Buffer
		require.NoError(t, deleteModuleRegistry(ctx, c, "catalog", &out))
		assert.Contains(t, out.String(), "catalog")

		k8sClient, err := c.GetClient()
		require.NoError(t, err)
		store := pkgmodule.NewStore(k8sClient)

		_, err = store.GetRegistry(ctx, "catalog")
		assert.True(t, apierrors.IsNotFound(err))

		remaining, err := store.GetRegistry(ctx, "mine")
		require.NoError(t, err)
		assert.Equal(t, "mine", remaining.Name)
	})

	t.Run("unknown name errors and changes nothing", func(t *testing.T) {
		c := moduleArgs(t)
		var discard bytes.Buffer
		require.NoError(t, addModuleRegistry(ctx, c, pkgaddon.Registry{
			Name: "catalog",
			Git:  &pkgaddon.GitAddonSource{URL: "https://github.com/kubevela/catalog", Path: "module"},
		}, false, &discard))

		var out bytes.Buffer
		err := deleteModuleRegistry(ctx, c, "nope", &out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nope")

		k8sClient, err2 := c.GetClient()
		require.NoError(t, err2)
		_, err2 = pkgmodule.NewStore(k8sClient).GetRegistry(ctx, "catalog")
		assert.NoError(t, err2, "the existing registry must be untouched")
	})

	t.Run("removes an entry ResolveRegistry would reject as unsupported", func(t *testing.T) {
		// delete must be able to clean up a bad entry, so it cannot go
		// through the strict ResolveRegistry the way get does.
		c := moduleArgs(t)
		var discard bytes.Buffer
		require.NoError(t, addModuleRegistry(ctx, c, pkgaddon.Registry{
			Name: "legacy",
			Helm: &pkgaddon.HelmSource{URL: "https://charts.example.com/legacy"},
		}, false, &discard))

		var out bytes.Buffer
		require.NoError(t, deleteModuleRegistry(ctx, c, "legacy", &out))
		assert.Contains(t, out.String(), "legacy")

		k8sClient, err := c.GetClient()
		require.NoError(t, err)
		_, err = pkgmodule.NewStore(k8sClient).GetRegistry(ctx, "legacy")
		assert.True(t, apierrors.IsNotFound(err))
	})
}
