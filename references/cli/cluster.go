/*
Copyright 2021 The KubeVela Authors.

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

package cli

import (
	"context"

	v1alpha12 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/a/preimport"
)

const (
	// FlagClusterName specifies the cluster name
	FlagClusterName = "name"
)

// ClusterCommandGroup create a group of cluster command
func ClusterCommandGroup(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Manage Clusters",
		Long:  "Manage Clusters",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
		// check if cluster-gateway is ready
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if c.Config == nil {
				if err := c.SetConfig(); err != nil {
					return errors.Wrapf(err, "failed to set config for k8s client")
				}
			}
			c.Config.Wrap(multicluster.NewSecretModeMultiClusterRoundTripper)
			c.Client = nil
			preimport.SuppressLogging()
			k8sClient, err := c.GetClient()
			preimport.ResumeLogging()
			if err != nil {
				return errors.Wrapf(err, "failed to get k8s client")
			}
			svc, err := multicluster.GetClusterGatewayService(context.Background(), k8sClient)
			if err != nil {
				return errors.Wrapf(err, "failed to get cluster secret namespace, please ensure cluster gateway is correctly deployed")
			}
			multicluster.ClusterGatewaySecretNamespace = svc.Namespace
			return nil
		},
	}
	cmd.AddCommand(
		NewClusterListCommand(&c),
		NewClusterJoinCommand(&c),
		NewClusterRenameCommand(&c),
		NewClusterDetachCommand(&c),
	)
	return cmd
}

// NewClusterListCommand create cluster list command
func NewClusterListCommand(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "list managed clusters",
		Long:    "list child clusters managed by KubeVela",
		Args:    cobra.ExactValidArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			secrets := v1.SecretList{}
			if err := c.Client.List(context.Background(), &secrets, client.HasLabels{v1alpha12.LabelKeyClusterCredentialType}, client.InNamespace(multicluster.ClusterGatewaySecretNamespace)); err != nil {
				return errors.Wrapf(err, "failed to get cluster secrets")
			}
			table := newUITable().AddRow("CLUSTER", "TYPE", "ENDPOINT")
			for _, secret := range secrets.Items {
				table.AddRow(secret.Name, secret.GetLabels()[v1alpha12.LabelKeyClusterCredentialType], string(secret.Data["endpoint"]))
			}
			if len(table.Rows) == 1 {
				cmd.Println("No managed cluster found.")
			} else {
				cmd.Println(table.String())
			}
			return nil
		},
	}
	return cmd
}

// NewClusterJoinCommand create command to help user join cluster to multicluster management
func NewClusterJoinCommand(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "join [KUBECONFIG]",
		Short: "join managed cluster",
		Long:  "join managed cluster by kubeconfig",
		Example: "# Join cluster declared in my-child-cluster.kubeconfig\n" +
			"> vela cluster join my-child-cluster.kubeconfig --name example-cluster",
		Args: cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// get ClusterName from flag or config
			clusterName, err := cmd.Flags().GetString(FlagClusterName)
			if err != nil {
				return errors.Wrapf(err, "failed to get cluster name flag")
			}
			cluster, err := multicluster.JoinClusterByKubeConfig(context.Background(), c.Client, args[0], clusterName)
			if err != nil {
				return err
			}
			cmd.Printf("Successfully add cluster %s, endpoint: %s.\n", clusterName, cluster.Server)
			return nil
		},
	}
	cmd.Flags().StringP(FlagClusterName, "n", "", "Specify the cluster name. If empty, it will use the cluster name in config file. Default to be empty.")
	return cmd
}

// NewClusterRenameCommand create command to help user rename cluster
func NewClusterRenameCommand(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename [OLD_NAME] [NEW_NAME]",
		Short: "rename managed cluster",
		Args:  cobra.ExactValidArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			oldClusterName := args[0]
			newClusterName := args[1]
			if err := multicluster.RenameCluster(context.Background(), c.Client, oldClusterName, newClusterName); err != nil {
				return err
			}
			cmd.Printf("Rename cluster %s to %s successfully.\n", oldClusterName, newClusterName)
			return nil
		},
	}
	return cmd
}

// NewClusterDetachCommand create command to help user detach existing cluster
func NewClusterDetachCommand(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "detach [CLUSTER_NAME]",
		Short: "detach managed cluster",
		Args:  cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]
			if err := multicluster.DetachCluster(context.Background(), c.Client, clusterName); err != nil {
				return err
			}
			cmd.Printf("Detach cluster %s successfully.\n", clusterName)
			return nil
		},
	}
	return cmd
}
