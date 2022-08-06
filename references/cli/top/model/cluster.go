package model

import (
	"context"
	"strings"

	"github.com/oam-dev/cluster-gateway/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/client"

	prismclusterv1alpha1 "github.com/kubevela/prism/pkg/apis/cluster/v1alpha1"
)

// Cluster is cluster resource struct
type Cluster struct {
	name        string
	alias       string
	clusterType string
	endpoint    string
	labels      string
}

// ClusterList is cluster resource list
type ClusterList struct {
	title []string
	data  []Cluster
}

// Header generate header of table in application view
func (l *ClusterList) Header() []string {
	return l.title
}

// Body generate body of table in application view
func (l *ClusterList) Body() [][]string {
	data := make([][]string, 0)
	for _, cluster := range l.data {
		data = append(data, []string{cluster.name, cluster.alias, cluster.clusterType, cluster.endpoint, cluster.labels})
	}
	return data
}

// ListClusters list clusters where application deploys resource
func ListClusters(ctx context.Context, c client.Client) *ClusterList {
	list := &ClusterList{
		title: []string{"Name", "Alias", "Type", "EndPoint", "Labels"},
	}
	name := ctx.Value(&CtxKeyAppName).(string)
	ns := ctx.Value(&CtxKeyNamespace).(string)
	app, err := LoadApplication(c, name, ns)
	if err != nil {
		return list
	}
	clusterSet := make(map[string]interface{})

	for _, svc := range app.Status.AppliedResources {
		if svc.Cluster == "" {
			clusterSet["local"] = struct{}{}
		} else {
			clusterSet[svc.Cluster] = struct{}{}
		}
	}

	clusters, _ := prismclusterv1alpha1.NewClusterClient(c).List(context.Background())

	for _, cluster := range clusters.Items {
		if _, ok := clusterSet[cluster.Name]; ok {
			clusterInfo := Cluster{
				name:        cluster.Name,
				alias:       cluster.Spec.Alias,
				clusterType: string(cluster.Spec.CredentialType),
				endpoint:    cluster.Spec.Endpoint,
			}
			var labels []string
			for k, v := range cluster.Labels {
				if !strings.HasPrefix(k, config.MetaApiGroupName) {
					labels = append(labels, "[blue::]"+k+"="+"[green::]"+v)
				}
			}
			clusterInfo.labels = strings.Join(labels, ",")
			list.data = append(list.data, clusterInfo)
		}
	}
	return list
}
