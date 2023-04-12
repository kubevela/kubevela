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
	"sort"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/fatih/color"
	"github.com/kubevela/pkg/util/runtime"
	"github.com/kubevela/pkg/util/slices"
	clustergatewayapi "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	"github.com/oam-dev/cluster-gateway/pkg/config"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
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

	// CreateLabel specifies the labels need to create in managedCluster
	CreateLabel = "labels"
)

// ClusterCommandGroup create a group of cluster command
func ClusterCommandGroup(f velacmd.Factory, c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Manage Kubernetes Clusters",
		Long:  "Manage Kubernetes Clusters for Continuous Delivery.",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		// check if cluster-gateway is ready
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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
		NewClusterLabelCommandGroup(&c),
		NewClusterAliasCommand(&c),
		NewClusterExportConfigCommand(f, ioStreams),
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
		Args:    cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			table := newUITable().AddRow("CLUSTER", "ALIAS", "TYPE", "ENDPOINT", "ACCEPTED", "LABELS")
			client, err := c.GetClient()
			if err != nil {
				return err
			}
			clusters, err := multicluster.NewClusterClient(client).List(context.Background())
			if err != nil {
				return errors.Wrap(err, "fail to get registered cluster")
			}
			for _, cluster := range clusters.Items {
				var labels []string
				for k, v := range cluster.Labels {
					if !strings.HasPrefix(k, config.MetaApiGroupName) {
						labels = append(labels, color.CyanString(k)+"="+color.GreenString(v))
					}
				}
				sort.Strings(labels)
				if len(labels) == 0 {
					labels = append(labels, "")
				}
				for i, l := range labels {
					if i == 0 {
						table.AddRow(cluster.Name, cluster.Spec.Alias, cluster.Spec.CredentialType, cluster.Spec.Endpoint, fmt.Sprintf("%v", cluster.Spec.Accepted), l)
					} else {
						table.AddRow("", "", "", "", "", l)
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
			"> vela cluster join my-child-cluster.kubeconfig --name example-cluster\n" +
			"> vela cluster join my-child-cluster.kubeconfig --name example-cluster --labels project=kubevela,owner=oam-dev",
		Args: cobra.ExactArgs(1),
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
			labels, err := cmd.Flags().GetString(CreateLabel)
			if err != nil {
				return errors.Wrapf(err, "failed to get label")
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
			ctx := context.WithValue(context.Background(), multicluster.KubeConfigContext, restConfig)
			clusterConfig, err := multicluster.JoinClusterByKubeConfig(ctx, client, managedClusterKubeConfig, clusterName,
				multicluster.JoinClusterCreateNamespaceOption(createNamespace),
				multicluster.JoinClusterEngineOption(clusterManagementType),
				multicluster.JoinClusterOCMOptions{
					InClusterBootstrap:     inClusterBootstrap,
					IoStreams:              ioStreams,
					HubConfig:              restConfig,
					TrackingSpinnerFactory: newTrackingSpinner,
				},
				multicluster.JoinClusterAlreadyExistCallback(func(name string) bool {
					if !NewUserInput().AskBool(fmt.Sprintf("Cluster %s already exists, do you want to overwrite it?", name), &UserInputOptions{AssumeYes: assumeYes}) {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Terminated.\n")
						return false
					}
					return true
				}))
			if err != nil {
				return err
			}
			cmd.Printf("Successfully add cluster %s, endpoint: %s.\n", clusterName, clusterConfig.Cluster.Server)

			if len(labels) > 0 {
				return addClusterLabels(cmd, c, clusterName, labels)
			}
			return nil
		},
	}
	cmd.Flags().StringP(FlagClusterName, "n", "", "Specify the cluster name. If empty, it will use the cluster name in config file. Default to be empty.")
	cmd.Flags().StringP(FlagClusterManagementEngine, "t", multicluster.ClusterGateWayEngine, "Specify the cluster management engine. If empty, it will use cluster-gateway cluster management solution. Default to be empty.")
	cmd.Flags().StringP(CreateNamespace, "", types.DefaultKubeVelaNS, "Specifies the namespace need to create in managedCluster")
	cmd.Flags().BoolP(FlagInClusterBootstrap, "", true, "If true, the registering managed cluster "+
		`will use the internal endpoint prescribed in the hub cluster's configmap "kube-public/cluster-info to register "`+
		"itself to the hub cluster. Otherwise use the original endpoint from the hub kubeconfig.")
	cmd.Flags().StringP(CreateLabel, "", "", "Specifies the labels need to create in managedCluster")

	return cmd
}

// NewClusterRenameCommand create command to help user rename cluster
func NewClusterRenameCommand(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename [OLD_NAME] [NEW_NAME]",
		Short: "rename managed cluster.",
		Long:  "rename managed cluster.",
		Args:  cobra.ExactArgs(2),
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
		Args:  cobra.ExactArgs(1),
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

// NewClusterAliasCommand create an alias to the named cluster
func NewClusterAliasCommand(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias CLUSTER_NAME ALIAS",
		Short: "alias a named cluster.",
		Long:  "alias a named cluster.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName, aliasName := args[0], args[1]
			k8sClient, err := c.GetClient()
			if err != nil {
				return err
			}
			if err = multicluster.AliasCluster(context.Background(), k8sClient, clusterName, aliasName); err != nil {
				return err
			}
			cmd.Printf("Alias cluster %s as %s.\n", clusterName, aliasName)
			return nil
		},
	}
	return cmd
}

// NewClusterProbeCommand create command to help user try health probe for existing cluster
// TODO(somefive): move prob logic into cluster management
func NewClusterProbeCommand(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "probe [CLUSTER_NAME]",
		Short: "health probe managed cluster.",
		Long:  "health probe managed cluster.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			content, err := multicluster.RequestRawK8sAPIForCluster(context.TODO(), "healthz", clusterName, config)
			if err != nil {
				return errors.Wrapf(err, "failed connect cluster %s", clusterName)
			}
			cmd.Printf("Connect to cluster %s successfully.\n%s\n", clusterName, string(content))
			return nil
		},
	}
	return cmd
}

// NewClusterLabelCommandGroup create a group of commands to manage cluster labels
func NewClusterLabelCommandGroup(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "labels",
		Short: "Manage Kubernetes Cluster Labels",
		Long:  "Manage Kubernetes Cluster Labels for Continuous Delivery.",
	}
	cmd.AddCommand(
		NewClusterAddLabelsCommand(c),
		NewClusterDelLabelsCommand(c),
	)
	return cmd
}

func updateClusterLabelAndPrint(cmd *cobra.Command, cli client.Client, vc *multicluster.VirtualCluster, clusterName string) (err error) {
	if err = cli.Update(context.Background(), vc.Object); err != nil {
		return errors.Errorf("failed to update labels for cluster %s, type: %s", vc.FullName(), vc.Type)
	}
	if vc, err = multicluster.GetVirtualCluster(context.Background(), cli, clusterName); err != nil {
		return errors.Wrapf(err, "failed to get updated cluster %s", clusterName)
	}
	cmd.Printf("Successfully update labels for cluster %s, type: %s.\n", vc.FullName(), vc.Type)
	if len(vc.Labels) == 0 {
		cmd.Println("No valid label exists.")
	}
	var keys []string
	for k := range vc.Labels {
		if !strings.HasPrefix(k, config.MetaApiGroupName) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		cmd.Println(color.CyanString(k) + "=" + color.GreenString(vc.Labels[k]))
	}
	return nil
}

// NewClusterAddLabelsCommand create command to add labels for managed cluster
func NewClusterAddLabelsCommand(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add CLUSTER_NAME LABELS",
		Short:   "add labels to managed cluster",
		Long:    "add labels to managed cluster",
		Example: "vela cluster labels add my-cluster project=kubevela,owner=oam-dev",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]
			labels := args[1]
			return addClusterLabels(cmd, c, clusterName, labels)
		},
	}
	return cmd
}

func addClusterLabels(cmd *cobra.Command, c *common.Args, clusterName, labels string) error {
	addLabels := map[string]string{}
	for _, kv := range strings.Split(labels, ",") {
		parts := strings.Split(kv, "=")
		if len(parts) != 2 {
			return errors.Errorf("invalid label key-value pair %s, should use the format LABEL_KEY=LABEL_VAL", kv)
		}
		addLabels[parts[0]] = parts[1]
	}

	cli, err := c.GetClient()
	if err != nil {
		return err
	}
	vc, err := multicluster.GetVirtualCluster(context.Background(), cli, clusterName)
	if err != nil {
		return errors.Wrapf(err, "failed to get cluster %s", clusterName)
	}
	if vc.Object == nil {
		return errors.Errorf("cluster type %s do not support add labels now", vc.Type)
	}
	meta.AddLabels(vc.Object, addLabels)
	return updateClusterLabelAndPrint(cmd, cli, vc, clusterName)
}

// NewClusterDelLabelsCommand create command to delete labels for managed cluster
func NewClusterDelLabelsCommand(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "del CLUSTER_NAME LABELS",
		Aliases: []string{"delete", "remove"},
		Short:   "delete labels for managed cluster",
		Long:    "delete labels for managed cluster",
		Args:    cobra.ExactArgs(2),
		Example: "vela cluster labels del my-cluster project,owner",
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]
			removeLabels := strings.Split(args[1], ",")
			cli, err := c.GetClient()
			if err != nil {
				return err
			}
			vc, err := multicluster.GetVirtualCluster(context.Background(), cli, clusterName)
			if err != nil {
				return errors.Wrapf(err, "failed to get cluster %s", clusterName)
			}
			if vc.Object == nil {
				return errors.Errorf("cluster type %s do not support delete labels now", vc.Type)
			}
			for _, l := range removeLabels {
				if _, found := vc.Labels[l]; !found {
					return errors.Errorf("no such label %s", l)
				}
			}
			meta.RemoveLabels(vc.Object, removeLabels...)
			return updateClusterLabelAndPrint(cmd, cli, vc, clusterName)
		},
	}
	return cmd
}

// NewClusterExportConfigCommand create command to export multi-cluster config
func NewClusterExportConfigCommand(f velacmd.Factory, ioStreams cmdutil.IOStreams) *cobra.Command {
	var labelSelector string
	cmd := &cobra.Command{
		Use:   "export-config",
		Short: i18n.T("Export multi-cluster kubeconfig"),
		Long: templates.LongDesc(i18n.T(`
			Export multi-cluster kubeconfig

			Load existing cluster kubeconfig and list clusters registered in
			KubeVela. Export the proxy access of these clusters to KubeConfig
			and print it out.
		`)),
		Example: templates.Examples(i18n.T(`
			# Export all clusters to kubeconfig
			vela cluster export-config

			# Export clusters with specified kubeconfig
			KUBECONFIG=./my-hub-cluster.kubeconfig vela cluster export-config

			# Export clusters with specified labels
			vela cluster export-config -l gpu-cluster=true

			# Export clusters to kubeconfig and save in file
			vela cluster export-config > my-vela.kubeconfig

			# Use the exported kubeconfig in kubectl
			KUBECONFIG=my-vela.kubeconfig kubectl get namespaces --cluster c2
		`)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := runtime.Must(clientcmd.NewDefaultClientConfigLoadingRules().Load())
			ctx, ok := cfg.Contexts[cfg.CurrentContext]
			if !ok {
				return fmt.Errorf("cannot find current context %s in given config", cfg.CurrentContext)
			}
			baseCluster, ok := cfg.Clusters[ctx.Cluster]
			if !ok {
				return fmt.Errorf("cannot find base cluster %s in given config", ctx.Cluster)
			}
			selector, err := labels.Parse(labelSelector)
			if err != nil {
				return fmt.Errorf("invalid selector %s: %w", labelSelector, err)
			}
			clusters, err := multicluster.NewClusterClient(f.Client()).List(cmd.Context(), client.MatchingLabelsSelector{Selector: selector})
			if err != nil {
				return fmt.Errorf("failed to load clusters: %w", err)
			}
			clusterNames := slices.Filter(
				slices.Map(clusters.Items, func(cluster clustergatewayapi.VirtualCluster) string { return cluster.Name }),
				func(s string) bool { return s != multicluster.ClusterLocalName })

			if len(clusterNames) == 0 {
				return fmt.Errorf("no cluster found")
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%d cluster loaded: [%s]\n", len(clusterNames), strings.Join(clusterNames, ", "))

			delete(cfg.Clusters, ctx.Cluster)
			ctx.Cluster = types.ClusterLocalName
			cfg.Clusters[types.ClusterLocalName] = baseCluster.DeepCopy()
			for _, clusterName := range clusterNames {
				cls := baseCluster.DeepCopy()
				cls.LocationOfOrigin = ""
				cls.Server = strings.Join([]string{cls.Server, "apis",
					clustergatewayapi.SchemeGroupVersion.Group,
					clustergatewayapi.SchemeGroupVersion.Version,
					"clustergateways", clusterName, "proxy"}, "/")
				cfg.Clusters[clusterName] = cls
			}
			bs, err := clientcmd.Write(*cfg)
			if err != nil {
				return fmt.Errorf("failed to marshal generated kubeconfig: %w", err)
			}
			_, _ = ioStreams.Out.Write(bs)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "kubeconfig generated.\n")
			return nil
		},
	}
	cmd.Flags().StringVarP(&labelSelector, "selector", "l", labelSelector, "LabelSelector for select clusters to export.")

	return velacmd.NewCommandBuilder(f, cmd).WithResponsiveWriter().Build()
}
