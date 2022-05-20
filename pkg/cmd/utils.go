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

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/cmd/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/env"
)

// GetNamespace get namespace from command flags and env
func GetNamespace(f Factory, cmd *cobra.Command) string {
	namespace, err := cmd.Flags().GetString(flagNamespace)
	cmdutil.CheckErr(err)
	if namespace != "" {
		return namespace
	}
	// find namespace from env
	envName, err := cmd.Flags().GetString(flagEnv)
	if err != nil {
		// ignore env if the command does not use the flag
		return ""
	}
	cmdutil.CheckErr(common.SetGlobalClient(f.Client()))
	var envMeta *types.EnvMeta
	if envName != "" {
		envMeta, err = env.GetEnvByName(envName)
	} else {
		envMeta, err = env.GetCurrentEnv()
	}
	if err != nil {
		return ""
	}
	return envMeta.Namespace
}

// GetCluster get cluster from command flags
func GetCluster(cmd *cobra.Command) string {
	cluster, err := cmd.Flags().GetString(flagCluster)
	cmdutil.CheckErr(err)
	if cluster == "" {
		return types.ClusterLocalName
	}
	return cluster
}

// GetClusters get cluster from command flags
func GetClusters(cmd *cobra.Command) []string {
	clusters, err := cmd.Flags().GetStringSlice(flagCluster)
	cmdutil.CheckErr(err)
	if len(clusters) == 0 {
		return []string{types.ClusterLocalName}
	}
	return clusters
}
