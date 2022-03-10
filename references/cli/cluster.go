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
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/oam-dev/cluster-gateway/pkg/config"
	"github.com/oam-dev/cluster-gateway/pkg/generated/clientset/versioned"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	// FlagClusterName specifies the cluster name
	FlagClusterName = "name"
	// FlagClusterManagementEngine specifies the cluster management type, eg: ocm
	FlagClusterManagementEngine = "engine"
	// FlagKubeConfigPath specifies the kubeconfig path
	FlagKubeConfigPath = "kubeconfig-path"
	// FlagInClusterBootstrap prescribes the cluster registration to use the internal
	// IP from the kube-public/cluster-info configmap, otherwise the endpoint in the
	// hub kubeconfig will be used for registration.
	FlagInClusterBootstrap = "in-cluster-boostrap"

	// CreateNamespace specifies the namespace need to create in managedCluster
	CreateNamespace = "create-namespace"
)

// ClusterCommandGroup create a group of cluster command
func ClusterCommandGroup(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Manage Kubernetes Clusters",
		Long:  "Manage Kubernetes Clusters for Continuous Delivery.",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		// check if cluster-gateway is ready
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			config.Wrap(multicluster.NewSecretModeMultiClusterRoundTripper)
			k8sClient, err := c.GetClient()
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
		NewClusterJoinCommand(&c, ioStreams),
		NewClusterRenameCommand(&c),
		NewClusterDetachCommand(&c),
		NewClusterProbeCommand(&c),
		NewClusterAddLabelsCommand(&c),
		NewClusterDelLabelsCommand(&c),
	)
	return cmd
}

// NewClusterListCommand create cluster list command
func NewClusterListCommand(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "list managed clusters",
		Long:    "list worker clusters managed by KubeVela.",
		Args:    cobra.ExactValidArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			table := newUITable().AddRow("CLUSTER", "TYPE", "ENDPOINT", "ACCEPTED", "LABELS")
			client, err := c.GetClient()
			if err != nil {
				return err
			}
			clusters, err := multicluster.ListVirtualClusters(context.Background(), client)
			if err != nil {
				return errors.Wrap(err, "fail to get registered cluster")
			}
			for _, cluster := range clusters {
				var labels []string
				for k, v := range cluster.Labels {
					if !strings.HasPrefix(k, config.MetaApiGroupName) {
						labels = append(labels, color.CyanString(k)+"="+color.GreenString(v))
					}
				}
				if len(labels) == 0 {
					labels = append(labels, "")
				}
				for i, l := range labels {
					if i == 0 {
						table.AddRow(cluster.Name, cluster.Type, cluster.EndPoint, fmt.Sprintf("%v", cluster.Accepted), l)
					} else {
						table.AddRow("", "", "", "", l)
					}
				}
			}
			if len(table.Rows) == 1 {
				cmd.Println("No cluster found.")
			} else {
				cmd.Println(table.String())
			}
			return nil
		},
	}
	return cmd
}

// NewClusterJoinCommand create command to help user join cluster to multicluster management
func NewClusterJoinCommand(c *common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "join [KUBECONFIG]",
		Short: "join managed cluster.",
		Long:  "join managed cluster by kubeconfig.",
		Example: "# Join cluster declared in my-child-cluster.kubeconfig\n" +
			"> vela cluster join my-child-cluster.kubeconfig --name example-cluster",
		Args: cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// get ClusterName from flag or config
			clusterName, err := cmd.Flags().GetString(FlagClusterName)
			if err != nil {
				return errors.Wrapf(err, "failed to get cluster name flag")
			}
			clusterManagementType, err := cmd.Flags().GetString(FlagClusterManagementEngine)
			if err != nil {
				return errors.Wrapf(err, "failed to get cluster management type flag")
			}
			// get need created namespace in managed cluster
			createNamespace, err := cmd.Flags().GetString(CreateNamespace)
			if err != nil {
				return errors.Wrapf(err, "failed to get create namespace")
			}
			client, err := c.GetClient()
			if err != nil {
				return err
			}
			restConfig, err := c.GetConfig()
			if err != nil {
				return err
			}

			var inClusterBootstrap *bool
			if _inClusterBootstrap, err := cmd.Flags().GetBool(FlagInClusterBootstrap); err == nil {
				inClusterBootstrap = pointer.Bool(_inClusterBootstrap)
			}

			managedClusterKubeConfig := args[0]
			clusterConfig, err := multicluster.JoinClusterByKubeConfig(context.Background(), client, managedClusterKubeConfig, clusterName,
				multicluster.JoinClusterCreateNamespaceOption(createNamespace),
				multicluster.JoinClusterEngineOption(clusterManagementType),
				multicluster.JoinClusterOCMOptions{
					InClusterBootstrap:     inClusterBootstrap,
					IoStreams:              ioStreams,
					HubConfig:              restConfig,
					TrackingSpinnerFactory: newTrackingSpinner,
				})
			if err != nil {
				return err
			}
			cmd.Printf("Successfully add cluster %s, endpoint: %s.\n", clusterName, clusterConfig.Cluster.Server)
			return nil
		},
	}
	cmd.Flags().StringP(FlagClusterName, "n", "", "Specify the cluster name. If empty, it will use the cluster name in config file. Default to be empty.")
	cmd.Flags().StringP(FlagClusterManagementEngine, "t", multicluster.ClusterGateWayEngine, "Specify the cluster management engine. If empty, it will use cluster-gateway cluster management solution. Default to be empty.")
	cmd.Flags().StringP(CreateNamespace, "", types.DefaultKubeVelaNS, "Specifies the namespace need to create in managedCluster")
	cmd.Flags().BoolP(FlagInClusterBootstrap, "", true, "If true, the registering managed cluster "+
		`will use the internal endpoint prescribed in the hub cluster's configmap "kube-public/cluster-info to register "`+
		"itself to the hub cluster. Otherwise use the original endpoint from the hub kubeconfig.")
	return cmd
}

// NewClusterRenameCommand create command to help user rename cluster
func NewClusterRenameCommand(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename [OLD_NAME] [NEW_NAME]",
		Short: "rename managed cluster.",
		Long:  "rename managed cluster.",
		Args:  cobra.ExactValidArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			oldClusterName := args[0]
			newClusterName := args[1]
			k8sClient, err := c.GetClient()
			if err != nil {
				return err
			}
			if err := multicluster.RenameCluster(context.Background(), k8sClient, oldClusterName, newClusterName); err != nil {
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
		Short: "detach managed cluster.",
		Long:  "detach managed cluster.",
		Args:  cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]
			configPath, _ := cmd.Flags().GetString(FlagKubeConfigPath)
			cli, err := c.GetClient()
			if err != nil {
				return err
			}
			err = multicluster.DetachCluster(context.Background(), cli, clusterName,
				multicluster.DetachClusterManagedClusterKubeConfigPathOption(configPath))
			if err != nil {
				return err
			}
			cmd.Printf("Detach cluster %s successfully.\n", clusterName)
			return nil
		},
	}
	cmd.Flags().StringP(FlagKubeConfigPath, "p", "", "Specify the kubeconfig path of managed cluster. If you use ocm to manage your cluster, you must set the kubeconfig-path.")
	return cmd
}

// NewClusterProbeCommand create command to help user try health probe for existing cluster
// TODO(somefive): move prob logic into cluster management
func NewClusterProbeCommand(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "probe [CLUSTER_NAME]",
		Short: "health probe managed cluster.",
		Long:  "health probe managed cluster.",
		Args:  cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]
			if clusterName == multicluster.ClusterLocalName {
				return errors.New("you must specify a remote cluster name")
			}
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			content, err := versioned.NewForConfigOrDie(config).ClusterV1alpha1().ClusterGateways().RESTClient(clusterName).Get().AbsPath("healthz").DoRaw(context.TODO())
			if err != nil {
				return errors.Wrapf(err, "failed connect cluster %s", clusterName)
			}
			cmd.Printf("Connect to cluster %s successfully.\n%s\n", clusterName, string(content))
			return nil
		},
	}
	return cmd
}

// NewClusterAddLabelsCommand create command to add labels for managed cluster
func NewClusterAddLabelsCommand(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use: "add-labels CLUSTER_NAME LABELS",
		Short: "add labels to managed cluster",
		Long: "add labels to managed cluster",
		Example: "vela cluster add-labels my-cluster project=kubevela,owner=oam-dev",
		RunE: func(cmd *cobra.Command, args []string) error {
			
		},
	}
	return cmd
}

// NewClusterDelLabelsCommand create command to delete labels for managed cluster
func NewClusterDelLabelsCommand(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use: "del-labels CLUSTER_NAME LABELS",
		Short: "delete labels for managed cluster",
		Long: "delete labels for managed cluster",
		Example: "vela cluster del-labels my-cluster project,owner",
		RunE: func(cmd *cobra.Command, args []string) error {

		},
	}
	return cmd
}