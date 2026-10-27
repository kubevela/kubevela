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
	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ocmclusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// ListClustersInput has no fields.
type ListClustersInput struct{}

// ClusterSummary is one cluster KubeVela can deploy to. Only metadata is
// read: the cluster Secret's data, which holds the credentials and the
// endpoint, never enters this process.
type ClusterSummary struct {
	Name  string `json:"name"`
	Alias string `json:"alias,omitempty"`
	// Type is the credential type recorded on the cluster, such as
	// X509Certificate or ServiceAccountToken, or Internal for the local
	// cluster. It says how the gateway authenticates, not what the secret is.
	Type         string            `json:"type"`
	EndpointType string            `json:"endpointType,omitempty"`
	Accepted     bool              `json:"accepted"`
	Local        bool              `json:"local"`
	Source       string            `json:"source"`
	Labels       map[string]string `json:"labels,omitempty"`
}

// ListClustersOutput is the structured result of list_clusters.
type ListClustersOutput struct {
	Clusters []ClusterSummary `json:"clusters" jsonschema:"Clusters with alias, credential type, endpoint and labels."`
	Evidence []string         `json:"evidence"`
	Meta     Meta             `json:"meta"`
}

func (d *deps) listClusters(ctx context.Context, _ *sdk.CallToolRequest, _ ListClustersInput) (*sdk.CallToolResult, ListClustersOutput, error) {
	out := ListClustersOutput{Clusters: []ClusterSummary{{
		Name:     multicluster.ClusterLocalName,
		Type:     string(v1alpha1.CredentialTypeInternal),
		Accepted: true,
		Local:    true,
		Source:   "control plane",
	}}}
	cli := d.f.Client()
	if cli == nil {
		return nil, out, errNoConfig
	}
	ns := multicluster.ClusterGatewaySecretNamespace
	secrets := &metav1.PartialObjectMetadataList{}
	secrets.SetGroupVersionKind(schema.GroupVersionKind{Group: corev1.GroupName, Version: "v1", Kind: "SecretList"})
	if err := cli.List(ctx, secrets, client.InNamespace(ns), client.HasLabels{clustercommon.LabelKeyClusterCredentialType}); err != nil {
		return nil, out, fmt.Errorf("list cluster secrets metadata in %s: %w", ns, err)
	}
	for i := range secrets.Items {
		s := &secrets.Items[i]
		out.Clusters = append(out.Clusters, ClusterSummary{
			Name:         s.Name,
			Alias:        text(s.Annotations[v1alpha1.AnnotationClusterAlias]),
			Type:         s.Labels[clustercommon.LabelKeyClusterCredentialType],
			EndpointType: s.Labels[clustercommon.LabelKeyClusterEndpointType],
			Accepted:     true,
			Source:       "cluster secret",
			Labels:       redactStringMap(s.Labels),
		})
	}
	out.Evidence = []string{fmt.Sprintf("%d Secret objects in namespace %s labelled %s, metadata only", len(secrets.Items), ns, clustercommon.LabelKeyClusterCredentialType)}

	var managed ocmclusterv1.ManagedClusterList
	switch err := cli.List(ctx, &managed); {
	case err == nil:
		for i := range managed.Items {
			mc := &managed.Items[i]
			out.Clusters = append(out.Clusters, ClusterSummary{
				Name:     mc.Name,
				Alias:    text(mc.Annotations[v1alpha1.AnnotationClusterAlias]),
				Type:     string(v1alpha1.CredentialTypeOCMManagedCluster),
				Accepted: mc.Spec.HubAcceptsClient,
				Source:   "ManagedCluster",
				Labels:   redactStringMap(mc.Labels),
			})
		}
		out.Evidence = append(out.Evidence, fmt.Sprintf("%d ManagedCluster objects", len(managed.Items)))
	case velaerrors.IsCRDNotExists(err):
	default:
		out.Meta.Warnings = append(out.Meta.Warnings, "ManagedCluster objects unavailable: "+redactLine(err.Error()))
	}

	sort.SliceStable(out.Clusters, func(i, j int) bool {
		if out.Clusters[i].Local != out.Clusters[j].Local {
			return out.Clusters[i].Local
		}
		return out.Clusters[i].Name < out.Clusters[j].Name
	})
	warnings := out.Meta.Warnings
	out.Meta = d.meta(ctx)
	out.Meta.Warnings = append(out.Meta.Warnings, warnings...)
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
	Addons   []AddonSummary `json:"addons" jsonschema:"Enabled addons."`
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
	if err := cli.List(ctx, &list, client.InNamespace(systemNamespace), client.HasLabels{oam.LabelAddonName, oam.LabelAddonVersion}, client.Limit(int64(maxListItems))); err != nil {
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
