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

package component

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	velatypes "github.com/oam-dev/kubevela/apis/types"
)

func TestGitFamilySource(t *testing.T) {
	assert.Equal(t, "git", GitFamilySource(Registry{Git: &GitAddonSource{URL: "https://github.com/o/r"}}))
	assert.Equal(t, "gitee", GitFamilySource(Registry{Gitee: &GiteeAddonSource{URL: "https://gitee.com/o/r"}}))
	assert.Equal(t, "gitlab", GitFamilySource(Registry{Gitlab: &GitlabAddonSource{URL: "https://gitlab.com", Repo: "o/r"}}))
	assert.Equal(t, "", GitFamilySource(Registry{Helm: &HelmSource{URL: "oci://reg.example.com/charts"}}))
	assert.Equal(t, "", GitFamilySource(Registry{OSS: &OSSAddonSource{Endpoint: "oss.example.com"}}))
	assert.Equal(t, "", GitFamilySource(Registry{}))
}

func TestGitSourceUnsupportedWording(t *testing.T) {
	detail := GitSourceUnsupportedDetail("gitee", ModuleGitRemedy)
	assert.Equal(t, "is a gitee source, and git registries are not supported for addon and module components; use an OCI registry", detail)

	err := GitSourceUnsupportedError("team-addons", "git", AddonGitRemedy)
	assert.ErrorIs(t, err, ErrGitSourceUnsupported, "callers classify the refusal with errors.Is")
	assert.Equal(t, `registry "team-addons" `+GitSourceUnsupportedDetail("git", AddonGitRemedy)+" ("+ErrGitSourceUnsupported.Error()+")", err.Error(),
		"the error and the admission detail must say the same thing")
}

// registryConfigMap builds the ConfigMap the registry store reads, holding
// the given records under the given name.
func registryConfigMap(t *testing.T, cmName string, registries map[string]Registry) *v1.ConfigMap {
	t.Helper()
	raw, err := json.Marshal(registries)
	require.NoError(t, err)
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: velatypes.DefaultKubeVelaNS},
		Data:       map[string]string{registriesKey: string(raw)},
	}
}

func fakeRegistryClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func TestListRegistryGitKinds(t *testing.T) {
	ctx := context.Background()

	t.Run("no registry ConfigMap", func(t *testing.T) {
		kinds, err := ListRegistryGitKinds(ctx, fakeRegistryClient(t))
		require.NoError(t, err, "a cluster with no registries configured is not an error")
		assert.Nil(t, kinds)
	})

	t.Run("mixed sources", func(t *testing.T) {
		cm := registryConfigMap(t, registryConfigMapName, map[string]Registry{
			"gh":     {Name: "gh", Git: &GitAddonSource{URL: "https://github.com/o/r"}},
			"gitee":  {Name: "gitee", Gitee: &GiteeAddonSource{URL: "https://gitee.com/o/r"}},
			"gl":     {Name: "gl", Gitlab: &GitlabAddonSource{URL: "https://gitlab.com", Repo: "o/r"}},
			"ecr":    {Name: "ecr", Helm: &HelmSource{URL: "oci://reg.example.com/addons"}},
			"museum": {Name: "museum", Helm: &HelmSource{URL: "https://charts.example.com"}},
			"oss":    {Name: "oss", OSS: &OSSAddonSource{Endpoint: "oss.example.com", Bucket: "b"}},
		})
		kinds, err := ListRegistryGitKinds(ctx, fakeRegistryClient(t, cm))
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"gh": "git", "gitee": "gitee", "gl": "gitlab"}, kinds,
			"only the Git family is reported; OCI, Helm and OSS entries are absent")
	})

	t.Run("nothing Git backed", func(t *testing.T) {
		cm := registryConfigMap(t, registryConfigMapName, map[string]Registry{
			"ecr": {Name: "ecr", Helm: &HelmSource{URL: "oci://reg.example.com/addons"}},
		})
		kinds, err := ListRegistryGitKinds(ctx, fakeRegistryClient(t, cm))
		require.NoError(t, err)
		assert.Nil(t, kinds, "the map is allocated lazily; the common case allocates nothing")
	})

	t.Run("unreadable records", func(t *testing.T) {
		cm := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: registryConfigMapName, Namespace: velatypes.DefaultKubeVelaNS},
			Data:       map[string]string{registriesKey: "{not json"},
		}
		_, err := ListRegistryGitKinds(ctx, fakeRegistryClient(t, cm))
		assert.Error(t, err, "a corrupt ConfigMap is reported, not treated as empty")
	})
}

func TestListRegistryGitKindsInReadsAnotherConfigMap(t *testing.T) {
	ctx := context.Background()
	modules := registryConfigMap(t, "vela-module-registry", map[string]Registry{
		"lab": {Name: "lab", Gitlab: &GitlabAddonSource{URL: "https://gitlab.com", Repo: "o/r"}},
	})
	addons := registryConfigMap(t, registryConfigMapName, map[string]Registry{
		"gh": {Name: "gh", Git: &GitAddonSource{URL: "https://github.com/o/r"}},
	})
	cli := fakeRegistryClient(t, modules, addons)

	kinds, err := ListRegistryGitKindsIn(ctx, cli, "vela-module-registry")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"lab": "gitlab"}, kinds, "the module ConfigMap is inspected, not the addon one")

	kinds, err = ListRegistryGitKindsIn(ctx, cli, "no-such-configmap")
	require.NoError(t, err)
	assert.Nil(t, kinds)
}

func TestListRegistrySources(t *testing.T) {
	ctx := context.Background()

	names, kinds, err := ListRegistrySources(ctx, fakeRegistryClient(t))
	require.NoError(t, err)
	assert.Empty(t, names)
	assert.Nil(t, kinds)

	cm := registryConfigMap(t, registryConfigMapName, map[string]Registry{
		"zeta":  {Name: "zeta", Helm: &HelmSource{URL: "oci://reg.example.com/addons"}},
		"alpha": {Name: "alpha", Git: &GitAddonSource{URL: "https://github.com/o/r"}},
		"mid":   {Name: "mid", Helm: &HelmSource{URL: "https://charts.example.com"}},
	})
	names, kinds, err = ListRegistrySources(ctx, fakeRegistryClient(t, cm))
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "mid", "zeta"}, names, "callers treat the order as a priority, so it must be stable")
	assert.Equal(t, map[string]string{"alpha": "git"}, kinds)

	bad := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: registryConfigMapName, Namespace: velatypes.DefaultKubeVelaNS},
		Data:       map[string]string{registriesKey: "{not json"},
	}
	_, _, err = ListRegistrySources(ctx, fakeRegistryClient(t, bad))
	assert.Error(t, err)
}
