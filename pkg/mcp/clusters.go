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

package mcp

import (
	"context"
	"fmt"
	"sort"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// ListClustersInput has no fields.
type ListClustersInput struct{}

// ClusterSummary is one cluster KubeVela can deploy to. Credentials are
// never read; the underlying secret's data stays on the API server.
type ClusterSummary struct {
	Name     string            `json:"name"`
	Alias    string            `json:"alias,omitempty"`
	Type     string            `json:"type"`
	Endpoint string            `json:"endpoint,omitempty"`
	Accepted bool              `json:"accepted"`
	Local    bool              `json:"local"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// ListClustersOutput is the structured result of list_clusters.
type ListClustersOutput struct {
	Clusters []ClusterSummary `json:"clusters"`
	Evidence []string         `json:"evidence"`
	Meta     Meta             `json:"meta"`
}

func (d *deps) listClusters(ctx context.Context, _ *sdk.CallToolRequest, _ ListClustersInput) (*sdk.CallToolResult, ListClustersOutput, error) {
	out := ListClustersOutput{Clusters: []ClusterSummary{}}
	cli := d.f.Client()
	if cli == nil {
		return nil, out, errNoConfig
	}
	clusters, err := multicluster.ListVirtualClusters(ctx, cli)
	if err != nil {
		return nil, out, fmt.Errorf("list clusters: %w", err)
	}
	for _, c := range clusters {
		out.Clusters = append(out.Clusters, ClusterSummary{
			Name:     c.Name,
			Alias:    c.Alias,
			Type:     string(c.Type),
			Endpoint: c.EndPoint,
			Accepted: c.Accepted,
			Local:    c.Name == multicluster.ClusterLocalName,
			Labels:   c.Labels,
		})
	}
	sort.SliceStable(out.Clusters, func(i, j int) bool {
		if out.Clusters[i].Local != out.Clusters[j].Local {
			return out.Clusters[i].Local
		}
		return out.Clusters[i].Name < out.Clusters[j].Name
	})
	out.Evidence = []string{fmt.Sprintf("cluster secrets in namespace %s and ManagedCluster objects, plus the local control plane", multicluster.ClusterGatewaySecretNamespace)}
	out.Meta = d.meta(ctx)
	return nil, out, nil
}

// ListAddonsInput has no fields.
type ListAddonsInput struct{}

// AddonSummary is one addon enabled on the cluster, read from the
// Application the addon installed.
type AddonSummary struct {
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Registry string   `json:"registry,omitempty"`
	Phase    string   `json:"phase"`
	Healthy  bool     `json:"healthy"`
	Clusters []string `json:"clusters,omitempty"`
	// Application is the namespace/name of the addon's Application, for
	// get_application and explain_application.
	Application string `json:"application"`
}

// ListAddonsOutput is the structured result of list_addons.
type ListAddonsOutput struct {
	// Addons lists enabled addons only. Registry catalogues are a network
	// call to systems outside the cluster and are not part of this tool.
	Addons   []AddonSummary `json:"addons"`
	Evidence []string       `json:"evidence"`
	Meta     Meta           `json:"meta"`
}

func (d *deps) listAddons(ctx context.Context, _ *sdk.CallToolRequest, _ ListAddonsInput) (*sdk.CallToolResult, ListAddonsOutput, error) {
	out := ListAddonsOutput{Addons: []AddonSummary{}}
	cli := d.f.Client()
	if cli == nil {
		return nil, out, errNoConfig
	}
	addons, err := enabledAddons(ctx, cli, d.opts.SystemNamespace)
	if err != nil {
		return nil, out, err
	}
	out.Addons = addons
	out.Evidence = []string{fmt.Sprintf("Applications in namespace %s labelled %s", d.opts.SystemNamespace, oam.LabelAddonName)}
	out.Meta = d.meta(ctx)
	return nil, out, nil
}

// enabledAddons reads the Applications addons install, the same source
// vela addon list uses for the enabled column.
func enabledAddons(ctx context.Context, cli client.Client, systemNamespace string) ([]AddonSummary, error) {
	var list v1beta1.ApplicationList
	if err := cli.List(ctx, &list, client.InNamespace(systemNamespace), client.HasLabels{oam.LabelAddonName, oam.LabelAddonVersion}); err != nil {
		return nil, fmt.Errorf("list addon applications in %s: %w", systemNamespace, err)
	}
	addons := []AddonSummary{}
	for i := range list.Items {
		app := &list.Items[i]
		name := app.Labels[oam.LabelAddonName]
		if name == "" {
			continue
		}
		clusters := map[string]bool{}
		for _, r := range app.Status.AppliedResources {
			clusters[clusterName(r.Cluster)] = true
		}
		a := AddonSummary{
			Name:        name,
			Version:     app.Labels[oam.LabelAddonVersion],
			Registry:    app.Labels[oam.LabelAddonRegistry],
			Phase:       string(app.Status.Phase),
			Healthy:     applicationHealthy(app),
			Application: app.Namespace + "/" + app.Name,
		}
		for c := range clusters {
			a.Clusters = append(a.Clusters, c)
		}
		sort.Strings(a.Clusters)
		addons = append(addons, a)
	}
	sort.Slice(addons, func(i, j int) bool { return addons[i].Name < addons[j].Name })
	return addons, nil
}
