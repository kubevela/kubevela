/*
Copyright 2022 The KubeVela Authors.

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

package model

import (
	"context"
	"strings"

	"github.com/oam-dev/cluster-gateway/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/multicluster"
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
type ClusterList []Cluster

// ListClusters list clusters where application deploys resource
func ListClusters(ctx context.Context, c client.Client) (ClusterList, error) {
	name := ctx.Value(&CtxKeyAppName).(string)
	ns := ctx.Value(&CtxKeyNamespace).(string)
	app, err := LoadApplication(c, name, ns)
	if err != nil {
		return ClusterList{}, err
	}
	clusterSet := make(map[string]interface{})

	for _, svc := range app.Status.AppliedResources {
		if svc.Cluster == "" {
			clusterSet[multicluster.ClusterLocalName] = struct{}{}
		} else {
			clusterSet[svc.Cluster] = struct{}{}
		}
	}

	list := make(ClusterList, 0)

	for key := range clusterSet {
		clusterInfo := Cluster{
			name: key,
		}
		cluster, err := multicluster.NewClusterClient(c).Get(context.Background(), key)
		if err != nil {
			continue
		} else {
			clusterInfo.alias = cluster.Spec.Alias
			clusterInfo.clusterType = string(cluster.Spec.CredentialType)
			clusterInfo.endpoint = cluster.Spec.Endpoint
			var labels []string
			for k, v := range cluster.Labels {
				if !strings.HasPrefix(k, config.MetaApiGroupName) {
					labels = append(labels, "[blue::]"+k+"="+"[green::]"+v)
				}
			}
			clusterInfo.labels = strings.Join(labels, ",")
		}
		list = append(list, clusterInfo)
	}
	return list, nil
}

// ToTableBody generate body of table in cluster view
func (l ClusterList) ToTableBody() [][]string {
	data := make([][]string, len(l)+1)
	data[0] = []string{AllCluster, "*", "*", "*", "*"}
	for index, cluster := range l {
		data[index+1] = []string{cluster.name, cluster.alias, cluster.clusterType, cluster.endpoint, cluster.labels}
	}
	return data
}
