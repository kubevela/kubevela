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

package module

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/registry/component"
)

func rawProps(t *testing.T, m map[string]interface{}) *runtime.RawExtension {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: b}
}

// gitKinds returns a lister reporting each of names as a Git-backed registry
// of the given kind, plus a counter of how many times it was called, so a test
// can assert the ConfigMap is read at most once per admission request.
func gitKinds(calls *int, kind string, names ...string) registryGitKindLister {
	kinds := map[string]string{}
	for _, n := range names {
		kinds[n] = kind
	}
	return func(context.Context, client.Client) (map[string]string, error) {
		*calls++
		return kinds, nil
	}
}

func failingKinds(calls *int, err error) registryGitKindLister {
	return func(context.Context, client.Client) (map[string]string, error) {
		*calls++
		return nil, err
	}
}

func moduleComponent(t *testing.T, name string, props map[string]interface{}) common.ApplicationComponent {
	t.Helper()
	return common.ApplicationComponent{Name: name, Type: ComponentType, Properties: rawProps(t, props)}
}

func TestValidateComponents(t *testing.T) {
	testCases := map[string]struct {
		components []common.ApplicationComponent
		lister     func(calls *int) registryGitKindLister
		wantFields []string
		wantReads  int
	}{
		"non-module components are ignored": {
			components: []common.ApplicationComponent{
				{Name: "api", Type: "webservice"},
				{Name: "addon", Type: "addon", Properties: rawProps(t, map[string]interface{}{"registry": "my-git"})},
			},
			lister: func(calls *int) registryGitKindLister { return gitKinds(calls, "git", "my-git") },
		},
		"a module component naming no registry needs no cluster read": {
			components: []common.ApplicationComponent{
				moduleComponent(t, "kit", map[string]interface{}{"module": "widget-kit", "version": "1.0.0"}),
			},
			lister: func(calls *int) registryGitKindLister { return gitKinds(calls, "git", "my-git") },
		},
		"a component with no properties at all is accepted": {
			components: []common.ApplicationComponent{{Name: "kit", Type: ComponentType}},
		},
		"empty properties are accepted": {
			components: []common.ApplicationComponent{{Name: "kit", Type: ComponentType, Properties: &runtime.RawExtension{}}},
		},
		"properties that do not decode are rejected": {
			components: []common.ApplicationComponent{
				moduleComponent(t, "kit", map[string]interface{}{"module": map[string]interface{}{"not": "a string"}}),
			},
			wantFields: []string{"spec.components[0].properties"},
		},
		"a numeric version does not decode into the version string": {
			components: []common.ApplicationComponent{
				moduleComponent(t, "kit", map[string]interface{}{"module": "widget-kit", "version": 2}),
			},
			wantFields: []string{"spec.components[0].properties"},
		},
		"a namespace property is forbidden": {
			components: []common.ApplicationComponent{
				moduleComponent(t, "kit", map[string]interface{}{"module": "widget-kit", "namespace": "kit-tenant"}),
			},
			wantFields: []string{"spec.components[0].properties.namespace"},
		},
		"an empty namespace property is still forbidden": {
			components: []common.ApplicationComponent{
				moduleComponent(t, "kit", map[string]interface{}{"module": "widget-kit", "namespace": ""}),
			},
			wantFields: []string{"spec.components[0].properties.namespace"},
		},
		"an OCI registry is accepted": {
			components: []common.ApplicationComponent{
				moduleComponent(t, "kit", map[string]interface{}{"module": "widget-kit", "registry": "ecr-modules"}),
			},
			lister:    func(calls *int) registryGitKindLister { return gitKinds(calls, "gitlab", "my-lab") },
			wantReads: 1,
		},
		"an unknown registry name is left to the controller": {
			components: []common.ApplicationComponent{
				moduleComponent(t, "kit", map[string]interface{}{"module": "widget-kit", "registry": "no-such-registry"}),
			},
			lister:    func(calls *int) registryGitKindLister { return gitKinds(calls, "git", "my-git") },
			wantReads: 1,
		},
		"a git-backed registry is rejected": {
			components: []common.ApplicationComponent{
				moduleComponent(t, "kit", map[string]interface{}{"module": "widget-kit", "registry": "my-git"}),
			},
			lister:     func(calls *int) registryGitKindLister { return gitKinds(calls, "git", "my-git") },
			wantFields: []string{"spec.components[0].properties.registry"},
			wantReads:  1,
		},
		"the ConfigMap is read once for many components": {
			components: []common.ApplicationComponent{
				moduleComponent(t, "a", map[string]interface{}{"module": "a", "registry": "ecr"}),
				moduleComponent(t, "b", map[string]interface{}{"module": "b", "registry": "my-git"}),
				{Name: "api", Type: "webservice"},
				moduleComponent(t, "c", map[string]interface{}{"module": "c", "registry": "my-gitee"}),
				moduleComponent(t, "d", map[string]interface{}{"module": "d"}),
			},
			lister:     func(calls *int) registryGitKindLister { return gitKinds(calls, "gitee", "my-git", "my-gitee") },
			wantFields: []string{"spec.components[1].properties.registry", "spec.components[3].properties.registry"},
			wantReads:  1,
		},
		"a failed registry read fails open, once": {
			components: []common.ApplicationComponent{
				moduleComponent(t, "a", map[string]interface{}{"module": "a", "registry": "my-git"}),
				moduleComponent(t, "b", map[string]interface{}{"module": "b", "registry": "my-git"}),
			},
			lister:    func(calls *int) registryGitKindLister { return failingKinds(calls, errors.New("configmap unreadable")) },
			wantReads: 1,
		},
		"a bad component does not hide a good one's registry check": {
			components: []common.ApplicationComponent{
				moduleComponent(t, "broken", map[string]interface{}{"version": []string{"1", "2"}}),
				moduleComponent(t, "kit", map[string]interface{}{"module": "widget-kit", "registry": "my-git"}),
			},
			lister:     func(calls *int) registryGitKindLister { return gitKinds(calls, "git", "my-git") },
			wantFields: []string{"spec.components[0].properties", "spec.components[1].properties.registry"},
			wantReads:  1,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			calls := 0
			v := NewValidator(nil)
			if tc.lister != nil {
				v.listRegistryGitKinds = tc.lister(&calls)
			} else {
				v.listRegistryGitKinds = failingKinds(&calls, errors.New("must not be read"))
			}
			app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: tc.components}}

			errs := v.ValidateComponents(context.Background(), app)

			var fields []string
			for _, e := range errs {
				fields = append(fields, e.Field)
			}
			assert.Equal(t, tc.wantFields, fields)
			assert.Equal(t, tc.wantReads, calls, "registry ConfigMap reads")
		})
	}
}

func TestValidateComponentsReportsTheOffendingValueAndRemedy(t *testing.T) {
	calls := 0
	v := NewValidator(nil)
	v.listRegistryGitKinds = gitKinds(&calls, "gitlab", "my-lab")
	app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
		moduleComponent(t, "kit", map[string]interface{}{"module": "widget-kit", "registry": "my-lab"}),
	}}}

	errs := v.ValidateComponents(context.Background(), app)
	require.Len(t, errs, 1)
	assert.Equal(t, "my-lab", errs[0].BadValue)
	assert.Equal(t, component.GitSourceUnsupportedDetail("gitlab", component.ModuleGitRemedy), errs[0].Detail,
		"the admission detail is the same sentence the resolver reports")
	assert.Contains(t, errs[0].Detail, "use an OCI registry")
	assert.NotContains(t, errs[0].Detail, "Helm repository", "modules never read a Helm chart repository")
}

func TestValidateComponentsExplainsTheRemovedNamespaceProperty(t *testing.T) {
	v := NewValidator(nil)
	app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
		moduleComponent(t, "kit", map[string]interface{}{"module": "widget-kit", "namespace": "kit-tenant"}),
	}}}

	errs := v.ValidateComponents(context.Background(), app)
	require.Len(t, errs, 1)
	assert.Equal(t, field.ErrorTypeForbidden, errs[0].Type)
	assert.Contains(t, errs[0].Detail, "namespace of the Application that installs it")
}

func TestValidateComponentsRedactsUndecodableProperties(t *testing.T) {
	v := NewValidator(nil)
	app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
		{Name: "kit", Type: ComponentType, Properties: &runtime.RawExtension{Raw: []byte(`{"module": "secret-token-abc"`)}},
	}}}

	errs := v.ValidateComponents(context.Background(), app)
	require.Len(t, errs, 1)
	assert.Equal(t, "<redacted>", errs[0].BadValue, "raw properties may carry credentials and are not echoed")
	assert.Contains(t, errs[0].Detail, "cannot be decoded as module component properties")
}

// TestValidatorReadsTheModuleRegistryConfigMap runs the production lister
// against a fake API server: the module registry ConfigMap, not the addon one,
// decides which names are Git backed.
func TestValidatorReadsTheModuleRegistryConfigMap(t *testing.T) {
	records := map[string]component.Registry{
		"my-lab": {Name: "my-lab", Gitlab: &component.GitlabAddonSource{URL: "https://gitlab.example.com", Repo: "team/modules"}},
		"ecr":    {Name: "ecr", Helm: &component.HelmSource{URL: "oci://123.dkr.ecr.us-west-2.amazonaws.com/module"}},
	}
	raw, err := json.Marshal(records)
	require.NoError(t, err)
	moduleCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: pkgmodule.ModuleRegistryConfigMap, Namespace: velatypes.DefaultKubeVelaNS},
		Data:       map[string]string{"registries": string(raw)},
	}
	// An addon registry of the same name is Git backed too; it must not be
	// what the module validator consults.
	addonRaw, err := json.Marshal(map[string]component.Registry{
		"ecr": {Name: "ecr", Git: &component.GitAddonSource{URL: "https://github.com/o/r"}},
	})
	require.NoError(t, err)
	addonCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "vela-addon-registry", Namespace: velatypes.DefaultKubeVelaNS},
		Data:       map[string]string{"registries": string(addonRaw)},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(moduleCM, addonCM).Build()
	v := NewValidator(cli)

	app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
		moduleComponent(t, "lab", map[string]interface{}{"module": "widget-kit", "registry": "my-lab"}),
		moduleComponent(t, "ecr", map[string]interface{}{"module": "widget-kit", "registry": "ecr"}),
	}}}
	errs := v.ValidateComponents(context.Background(), app)
	require.Len(t, errs, 1)
	assert.Equal(t, "spec.components[0].properties.registry", errs[0].Field)
	assert.Contains(t, errs[0].Detail, "is a gitlab source")

	t.Run("no module registry ConfigMap at all", func(t *testing.T) {
		empty := NewValidator(fake.NewClientBuilder().WithScheme(scheme).Build())
		assert.Empty(t, empty.ValidateComponents(context.Background(), app), "nothing configured means nothing is Git backed")
	})
}
