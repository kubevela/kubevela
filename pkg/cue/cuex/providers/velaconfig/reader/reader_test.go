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

package reader

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/velaconfig"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const plainTemplate = `
metadata: name: "endpoint"
template: parameter: url: string
`

const outputsTemplate = `
metadata: name: "database"
template: {
	outputs: settings: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {name: context.name + "-settings", namespace: context.namespace}
		data: host: parameter.host
	}
	parameter: host: string
}
`

const sensitiveTemplate = `
metadata: {name: "token", sensitive: true}
template: parameter: token: string
`

// withConfig stores a Config made from the template in a fake cluster, and
// returns that cluster's client.
func withConfig(t *testing.T, template, name string, props map[string]interface{}) client.Client {
	t.Helper()
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	factory := config.NewConfigFactory(cli)
	tmpl, err := factory.ParseTemplate(ctx, "", []byte(template))
	require.NoError(t, err)
	require.NoError(t, factory.CreateOrUpdateConfigTemplate(ctx, "vela-system", tmpl))
	cfg, err := factory.ParseConfig(ctx,
		config.NamespacedName{Name: tmpl.Name, Namespace: "vela-system"},
		config.Metadata{NamespacedName: config.NamespacedName{Name: name, Namespace: "default"}, Properties: props})
	require.NoError(t, err)
	require.NoError(t, factory.CreateOrUpdateConfig(ctx, cfg, "default"))
	return cli
}

func TestReadConfigWithoutOutputs(t *testing.T) {
	cli := withConfig(t, plainTemplate, "api", map[string]interface{}{"url": "https://api.example.com"})

	got, err := Config{Client: cli}.ReadConfig(context.Background(), "default", "api")
	require.NoError(t, err)
	require.Equal(t, &velaconfig.ReadResult{
		Properties: map[string]interface{}{"url": "https://api.example.com"},
		Template:   velaconfig.TemplateRef{Name: "endpoint", Namespace: "vela-system"},
		Output:     velaconfig.ObjectRef{APIVersion: "v1", Kind: "Secret", Name: "api", Namespace: "default"},
		Outputs:    map[string]velaconfig.ObjectRef{},
	}, got)
}

func TestReadConfigNamesOutputsByTheirTemplateLabel(t *testing.T) {
	cli := withConfig(t, outputsTemplate, "db", map[string]interface{}{"host": "db.local"})

	got, err := Config{Client: cli}.ReadConfig(context.Background(), "default", "db")
	require.NoError(t, err)
	require.Equal(t, map[string]velaconfig.ObjectRef{
		"settings": {APIVersion: "v1", Kind: "ConfigMap", Name: "db-settings", Namespace: "default"},
	}, got.Outputs)
}

func TestReadConfigKeysOutputsByKindAndNameWhenTheTemplateCannotRender(t *testing.T) {
	cli := withConfig(t, outputsTemplate, "db", map[string]interface{}{"host": "db.local"})
	var templates corev1.ConfigMapList
	require.NoError(t, cli.List(context.Background(), &templates, client.InNamespace("vela-system")))
	for i := range templates.Items {
		require.NoError(t, cli.Delete(context.Background(), &templates.Items[i]))
	}

	got, err := Config{Client: cli}.ReadConfig(context.Background(), "default", "db")
	require.NoError(t, err)
	require.Equal(t, map[string]velaconfig.ObjectRef{
		"ConfigMap/db-settings": {APIVersion: "v1", Kind: "ConfigMap", Name: "db-settings", Namespace: "default"},
	}, got.Outputs)
}

func TestReadConfigRefusesASensitiveConfig(t *testing.T) {
	cli := withConfig(t, sensitiveTemplate, "ci", map[string]interface{}{"token": "s3cr3t"})

	_, err := Config{Client: cli}.ReadConfig(context.Background(), "default", "ci")
	require.ErrorIs(t, err, config.ErrSensitiveConfig)
}

func TestReadConfigOfAMissingConfig(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()

	_, err := Config{Client: cli}.ReadConfig(context.Background(), "default", "nope")
	require.True(t, kerrors.IsNotFound(err), "got %v", err)
}
