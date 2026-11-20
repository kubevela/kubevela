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
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	clusterv1alpha1 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utiljson "k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/multicluster"
)

// RenderedAddon is what enabling an addon would install.
type RenderedAddon struct {
	Application map[string]any
	// Components are the Application's components, by name.
	Components map[string]map[string]any
	// Auxiliaries are the objects installed beside the Application.
	Auxiliaries []map[string]any
	// Definitions are keyed by kind, then name.
	Definitions map[string]map[string]map[string]any
	// ConfigTemplates, Schemas and Views are keyed by name.
	ConfigTemplates map[string]map[string]any
	Schemas         map[string]map[string]any
	Views           map[string]map[string]any
}

// RenderAddon renders the addon in dir with the given parameters, through
// the same functions `vela addon enable` uses, without installing anything:
// its godef/ Go definitions are compiled in and it is validated, and a Go
// definition named as a CUE one is refused, as without --override-definitions.
// The hub it renders against has registered every cluster parameter.clusters
// names.
func RenderAddon(dir string, parameter map[string]any) (*RenderedAddon, error) {
	pkg, err := addon.PrepareLocalInstallPackage(context.Background(), filepath.Base(dir), dir, false)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithObjects(clusterSecrets(parameter)...).Build()
	app, auxiliaries, err := addon.RenderApp(ctx, pkg, cli, parameter)
	if err != nil {
		return nil, err
	}
	out := &RenderedAddon{Components: map[string]map[string]any{}}
	if out.Application, err = toMap(app); err != nil {
		return nil, err
	}
	for _, comp := range app.Spec.Components {
		if out.Components[comp.Name], err = toMap(comp); err != nil {
			return nil, err
		}
	}
	for _, obj := range auxiliaries {
		out.Auxiliaries = append(out.Auxiliaries, obj.Object)
	}
	definitions, err := addon.RenderDefinitions(pkg, nil)
	if err != nil {
		return nil, err
	}
	configTemplates, err := addon.RenderConfigTemplates(ctx, pkg, cli)
	if err != nil {
		return nil, err
	}
	schemas, err := addon.RenderDefinitionSchema(pkg)
	if err != nil {
		return nil, err
	}
	views, err := addon.RenderViews(ctx, pkg)
	if err != nil {
		return nil, err
	}
	if out.Definitions, err = byKindAndName(definitions); err != nil {
		return nil, err
	}
	for _, into := range []struct {
		objs []*unstructured.Unstructured
		to   *map[string]map[string]any
	}{
		{configTemplates, &out.ConfigTemplates},
		{schemas, &out.Schemas},
		{views, &out.Views},
	} {
		if *into.to, err = byName(into.objs); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// clusterSecrets register the clusters parameter.clusters names, as
// cluster-gateway reads them, so a legacy addon deploying to runtime
// clusters renders as on a hub that has them.
func clusterSecrets(parameter map[string]any) []client.Object {
	names, _ := parameter[types.ClustersArg].([]any)
	var secrets []client.Object
	for _, n := range names {
		name, _ := n.(string)
		name = strings.TrimSpace(name)
		if name == "" || name == multicluster.ClusterLocalName {
			continue
		}
		secrets = append(secrets, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: multicluster.ClusterGatewaySecretNamespace,
			Labels: map[string]string{
				clustercommon.LabelKeyClusterCredentialType: string(clusterv1alpha1.CredentialTypeX509Certificate),
				clustercommon.LabelKeyClusterEndpointType:   string(clusterv1alpha1.ClusterEndpointTypeConst),
			},
		}})
	}
	return secrets
}

func (r *RenderedAddon) result() map[string]any {
	auxiliaries := make([]any, 0, len(r.Auxiliaries))
	for _, a := range r.Auxiliaries {
		auxiliaries = append(auxiliaries, a)
	}
	return map[string]any{
		"application":     r.Application,
		"components":      objects(r.Components),
		"auxiliaries":     auxiliaries,
		"definitions":     byKind(r.Definitions),
		"configTemplates": objects(r.ConfigTemplates),
		"schemas":         objects(r.Schemas),
		"views":           objects(r.Views),
	}
}

// byName keys objects of one kind by name.
func byName(objs []*unstructured.Unstructured) (map[string]map[string]any, error) {
	out := make(map[string]map[string]any, len(objs))
	for _, o := range objs {
		if _, clash := out[o.GetName()]; clash {
			return nil, fmt.Errorf("the addon ships two %ss named %q", o.GetKind(), o.GetName())
		}
		out[o.GetName()] = o.Object
	}
	return out, nil
}

// byKindAndName keys definitions by kind, then name: names are unique only
// within a kind.
func byKindAndName(objs []*unstructured.Unstructured) (map[string]map[string]map[string]any, error) {
	kinds := map[string][]*unstructured.Unstructured{}
	for _, o := range objs {
		kinds[o.GetKind()] = append(kinds[o.GetKind()], o)
	}
	out := make(map[string]map[string]map[string]any, len(kinds))
	for kind, of := range kinds {
		named, err := byName(of)
		if err != nil {
			return nil, err
		}
		out[kind] = named
	}
	return out, nil
}

func byKind(m map[string]map[string]map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = objects(v)
	}
	return out
}

func toMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encoding %T: %w", v, err)
	}
	// util/json keeps whole numbers as int64, as Kubernetes objects hold them.
	var out map[string]any
	if err := utiljson.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("decoding %T: %w", v, err)
	}
	return out, nil
}
