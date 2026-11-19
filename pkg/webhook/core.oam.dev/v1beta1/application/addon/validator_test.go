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

package addon

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func rawProps(t *testing.T, m map[string]interface{}) *runtime.RawExtension {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: b}
}

// registries returns a lister over the given names, plus a counter of how many
// times it was called, so a test can assert the ConfigMap is read at most once
// per admission request regardless of how many components name a registry.
func registries(calls *int, names ...string) registryLister {
	return func(context.Context, client.Client) ([]string, map[string]string, error) {
		*calls++
		return names, nil, nil
	}
}

// gitRegistries returns a lister over names where each of gitNames is reported
// as a git-backed entry. The names list covers both, because a git registry is
// configured and so must pass the unknown-name check before the source check
// can be the thing that rejects it.
func gitRegistries(calls *int, gitNames []string, names ...string) registryLister {
	kinds := map[string]string{}
	for _, n := range gitNames {
		kinds[n] = "git"
	}
	return func(context.Context, client.Client) ([]string, map[string]string, error) {
		*calls++
		return names, kinds, nil
	}
}

func failingRegistries(calls *int, err error) registryLister {
	return func(context.Context, client.Client) ([]string, map[string]string, error) {
		*calls++
		return nil, nil, err
	}
}

func TestValidateComponents(t *testing.T) {
	testCases := map[string]struct {
		components []common.ApplicationComponent
		lister     func(calls *int) registryLister
		wantFields []string
		wantReads  int
	}{
		"non-addon components are ignored": {
			components: []common.ApplicationComponent{
				{Name: "api", Type: "webservice"},
				{Name: "queue", Type: "worker"},
			},
		},
		"an addon component naming no registry needs no cluster read": {
			components: []common.ApplicationComponent{
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "fluxcd", "version": "1.0.0"})},
			},
		},
		"a component with no properties at all is accepted": {
			components: []common.ApplicationComponent{
				{Name: "velaux", Type: ComponentType},
			},
		},
		"properties that do not decode are rejected": {
			components: []common.ApplicationComponent{
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": map[string]interface{}{"not": "a string"}})},
			},
			wantFields: []string{"spec.components[0].properties"},
		},
		"a numeric version does not decode into the version string": {
			components: []common.ApplicationComponent{
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "fluxcd", "version": 2})},
			},
			wantFields: []string{"spec.components[0].properties"},
		},
		// Pinned-version resolution compares versions as strings, and an OSS
		// registry's metadata.yaml version need not be semver, so a version
		// that is not semver is not a version that cannot resolve.
		"a non-semver version is accepted": {
			components: []common.ApplicationComponent{
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "fluxcd", "version": "dev"})},
			},
		},
		"a v-prefixed version is accepted": {
			components: []common.ApplicationComponent{
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "fluxcd", "version": "v1.2.3"})},
			},
		},
		"a configured registry is accepted": {
			components: []common.ApplicationComponent{
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "fluxcd", "registry": "KubeVela"})},
			},
			lister:    func(calls *int) registryLister { return registries(calls, "KubeVela", "my-addons") },
			wantReads: 1,
		},
		"a git-backed registry is rejected": {
			components: []common.ApplicationComponent{
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "fluxcd", "registry": "my-git"})},
			},
			lister: func(calls *int) registryLister {
				return gitRegistries(calls, []string{"my-git"}, "KubeVela", "my-git")
			},
			wantFields: []string{"spec.components[0].properties.registry"},
			wantReads:  1,
		},
		"a registry that is not configured is rejected": {
			components: []common.ApplicationComponent{
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "fluxcd", "registry": "typo"})},
			},
			lister:     func(calls *int) registryLister { return registries(calls, "KubeVela") },
			wantFields: []string{"spec.components[0].properties.registry"},
			wantReads:  1,
		},
		// An empty list is a definite answer, not a missing one: no registries
		// are configured, so no registry name can resolve.
		"a named registry is rejected when no registries are configured": {
			components: []common.ApplicationComponent{
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "fluxcd", "registry": "my-addons"})},
			},
			lister:     func(calls *int) registryLister { return registries(calls) },
			wantFields: []string{"spec.components[0].properties.registry"},
			wantReads:  1,
		},
		"a failed registry read admits the component": {
			components: []common.ApplicationComponent{
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "fluxcd", "registry": "my-addons"})},
			},
			lister: func(calls *int) registryLister {
				return failingRegistries(calls, errors.New("configmap unavailable"))
			},
			wantReads: 1,
		},
		"the registry list is read once for several components": {
			components: []common.ApplicationComponent{
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "fluxcd", "registry": "KubeVela"})},
				{Name: "velaux", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "velaux", "registry": "KubeVela"})},
				{Name: "terraform", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "terraform", "registry": "KubeVela"})},
			},
			lister:    func(calls *int) registryLister { return registries(calls, "KubeVela") },
			wantReads: 1,
		},
		"each component is reported against its own index": {
			components: []common.ApplicationComponent{
				{Name: "api", Type: "webservice"},
				{Name: "fluxcd", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "fluxcd", "registry": "KubeVela"})},
				{Name: "velaux", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{"addon": "velaux", "registry": "typo"})},
			},
			lister:     func(calls *int) registryLister { return registries(calls, "KubeVela") },
			wantFields: []string{"spec.components[2].properties.registry"},
			wantReads:  1,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			reads := 0
			validator := &Validator{}
			if tc.lister != nil {
				validator.listRegistries = tc.lister(&reads)
			} else {
				validator.listRegistries = func(context.Context, client.Client) ([]string, map[string]string, error) {
					reads++
					t.Error("registry names were read for a component that names no registry")
					return nil, nil, nil
				}
			}
			app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: tc.components}}

			errs := validator.ValidateComponents(context.Background(), app)

			fields := make([]string, 0, len(errs))
			for _, err := range errs {
				fields = append(fields, err.Field)
			}
			assert.Equal(t, tc.wantFields, nilIfEmpty(fields))
			assert.Equal(t, tc.wantReads, reads, "unexpected registry read count")
		})
	}
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

// The rejection has to name the value the author wrote, so they can find it in
// their own YAML, and the detail has to name the values that would have worked.
func TestValidateComponentsReportsTheOffendingValue(t *testing.T) {
	reads := 0
	validator := &Validator{listRegistries: registries(&reads, "KubeVela", "my-modules")}
	app := &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
		{Name: "api", Type: "webservice"},
		{Name: "installer", Type: ComponentType, Properties: rawProps(t, map[string]interface{}{
			"addon": "fluxcd", "registry": "my-addons",
		})},
	}}}

	errs := validator.ValidateComponents(context.Background(), app)

	require.Len(t, errs, 1)
	assert.Equal(t, "spec.components[1].properties.registry", errs[0].Field)
	assert.Equal(t, "my-addons", errs[0].BadValue)
	assert.Contains(t, errs[0].Detail, "KubeVela")
	assert.Contains(t, errs[0].Detail, "my-modules")
}
