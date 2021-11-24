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

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	v13 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types2 "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ocmclusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha12 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	"github.com/oam-dev/cluster-gateway/pkg/generated/clientset/versioned"
	"github.com/oam-dev/cluster-register/pkg/hub"
	"github.com/oam-dev/cluster-register/pkg/spoke"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/clustermanager"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/a/preimport"
)

const (
	// FlagClusterName specifies the cluster name
	FlagClusterName = "name"
	// FlagClusterManagementEngine specifies the cluster management type, eg: ocm
	FlagClusterManagementEngine = "engine"
	// FlagKubeConfigPath specifies the kubeconfig path
	FlagKubeConfigPath = "kubeconfig-path"

	// ClusterGateWayClusterManagement cluster-gateway cluster management solution
	ClusterGateWayClusterManagement = "cluster-gateway"
	// OCMClusterManagement ocm cluster management solution
	OCMClusterManagement = "ocm"
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
		NewClusterProbeCommand(&c),
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
			table := newUITable().AddRow("CLUSTER", "TYPE", "ENDPOINT")
			clusters, err := clustermanager.GetRegisteredClusters(c.Client)
			if err != nil {
				return errors.Wrap(err, "fail to get registered cluster")
			}
			for _, cluster := range clusters {
				table.AddRow(cluster.Name, cluster.Type, cluster.EndPoint)
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

func ensureResourceTrackerCRDInstalled(c client.Client, clusterName string) error {
	ctx := context.Background()
	remoteCtx := multicluster.ContextWithClusterName(ctx, clusterName)
	crdName := types2.NamespacedName{Name: "resourcetrackers." + v1beta1.Group}
	if err := c.Get(remoteCtx, crdName, &v13.CustomResourceDefinition{}); err != nil {
		if !errors2.IsNotFound(err) {
			return errors.Wrapf(err, "failed to check resourcetracker crd in cluster %s", clusterName)
		}
		crd := &v13.CustomResourceDefinition{}
		if err = c.Get(ctx, crdName, crd); err != nil {
			return errors.Wrapf(err, "failed to get resourcetracker crd in hub cluster")
		}
		crd.ObjectMeta = v12.ObjectMeta{
			Name:        crdName.Name,
			Annotations: crd.Annotations,
			Labels:      crd.Labels,
		}
		if err = c.Create(remoteCtx, crd); err != nil {
			return errors.Wrapf(err, "failed to create resourcetracker crd in cluster %s", clusterName)
		}
	}
	return nil
}

func ensureVelaSystemNamespaceInstalled(c client.Client, clusterName string) error {
	ctx := context.Background()
	remoteCtx := multicluster.ContextWithClusterName(ctx, clusterName)
	if err := c.Get(remoteCtx, types2.NamespacedName{Name: types.DefaultKubeVelaNS}, &v1.Namespace{}); err != nil {
		if !errors2.IsNotFound(err) {
			return errors.Wrapf(err, "failed to check vela-system ")
		}
		if err = c.Create(remoteCtx, &v1.Namespace{ObjectMeta: v12.ObjectMeta{Name: types.DefaultKubeVelaNS}}); err != nil {
			return errors.Wrapf(err, "failed to create vela-system namespace")
		}
	}
	return nil
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
			config, err := clientcmd.LoadFromFile(args[0])
			if err != nil {
				return errors.Wrapf(err, "failed to get kubeconfig")
			}
			if len(config.CurrentContext) == 0 {
				return fmt.Errorf("current-context is not set")
			}
			ctx, ok := config.Contexts[config.CurrentContext]
			if !ok {
				return fmt.Errorf("current-context %s not found", config.CurrentContext)
			}
			cluster, ok := config.Clusters[ctx.Cluster]
			if !ok {
				return fmt.Errorf("cluster %s not found", ctx.Cluster)
			}
			authInfo, ok := config.AuthInfos[ctx.AuthInfo]
			if !ok {
				return fmt.Errorf("authInfo %s not found", ctx.AuthInfo)
			}
			// get ClusterName from flag or config
			clusterName, err := cmd.Flags().GetString(FlagClusterName)
			if err != nil {
				return errors.Wrapf(err, "failed to get cluster name flag")
			}
			if clusterName == "" {
				clusterName = ctx.Cluster
			}
			if clusterName == multicluster.ClusterLocalName {
				return fmt.Errorf("cannot use `%s` as cluster name, it is reserved as the local cluster", multicluster.ClusterLocalName)
			}

			clusterManagementType, err := cmd.Flags().GetString(FlagClusterManagementEngine)
			if err != nil {
				return errors.Wrapf(err, "failed to get cluster management type flag")
			}

			if clusterManagementType == "" {
				clusterManagementType = ClusterGateWayClusterManagement
			}

			switch clusterManagementType {
			case ClusterGateWayClusterManagement:
				if endpoint, err := utils.ParseAPIServerEndpoint(cluster.Server); err == nil {
					cluster.Server = endpoint
				} else {
					_, _ = cmd.OutOrStdout().Write([]byte("failed to parse server endpoint: " + err.Error()))
				}
				if err = registerClusterManagedByVela(c.Client, cluster, authInfo, clusterName); err != nil {
					return err
				}
			case OCMClusterManagement:
				if err = registerClusterManagedByOCM(c.Config, config, clusterName); err != nil {
					return err
				}
			}
			cmd.Printf("Successfully add cluster %s, endpoint: %s.\n", clusterName, cluster.Server)
			return nil
		},
	}
	cmd.Flags().StringP(FlagClusterName, "n", "", "Specify the cluster name. If empty, it will use the cluster name in config file. Default to be empty.")
	cmd.Flags().StringP(FlagClusterManagementEngine, "t", "", "Specify the cluster management engine. If empty, it will use cluster-gateway cluster management solution. Default to be empty.")
	return cmd
}

func registerClusterManagedByVela(k8sClient client.Client, cluster *clientcmdapi.Cluster, authInfo *clientcmdapi.AuthInfo, clusterName string) error {
	if err := clustermanager.EnsureClusterNotExists(k8sClient, clusterName); err != nil {
		return errors.Wrapf(err, "cannot use cluster name %s", clusterName)
	}
	var credentialType v1alpha12.CredentialType
	data := map[string][]byte{
		"endpoint": []byte(cluster.Server),
		"ca.crt":   cluster.CertificateAuthorityData,
	}
	if len(authInfo.Token) > 0 {
		credentialType = v1alpha12.CredentialTypeServiceAccountToken
		data["token"] = []byte(authInfo.Token)
	} else {
		credentialType = v1alpha12.CredentialTypeX509Certificate
		data["tls.crt"] = authInfo.ClientCertificateData
		data["tls.key"] = authInfo.ClientKeyData
	}
	secret := &v1.Secret{
		ObjectMeta: v12.ObjectMeta{
			Name:      clusterName,
			Namespace: multicluster.ClusterGatewaySecretNamespace,
			Labels: map[string]string{
				v1alpha12.LabelKeyClusterCredentialType: string(credentialType),
			},
		},
		Type: v1.SecretTypeOpaque,
		Data: data,
	}
	if err := k8sClient.Create(context.Background(), secret); err != nil {
		return errors.Wrapf(err, "failed to add cluster to kubernetes")
	}
	if err := ensureResourceTrackerCRDInstalled(k8sClient, clusterName); err != nil {
		_ = k8sClient.Delete(context.Background(), secret)
		return errors.Wrapf(err, "failed to ensure resourcetracker crd installed in cluster %s", clusterName)
	}
	if err := ensureVelaSystemNamespaceInstalled(k8sClient, clusterName); err != nil {
		_ = k8sClient.Delete(context.Background(), secret)
		return errors.Wrapf(err, "failed to ensure vela-system namespace installed in cluster %s", clusterName)
	}
	return nil
}

func registerClusterManagedByOCM(hubConfig *rest.Config, spokeConfig *clientcmdapi.Config, clusterName string) error {
	ctx := context.Background()
	hubCluster, err := hub.NewHubCluster(hubConfig)
	if err != nil {
		return errors.Wrap(err, "fail to create client connect to hub cluster")
	}

	crdName := types2.NamespacedName{Name: "managedclusters." + ocmclusterv1.GroupName}
	if err := hubCluster.Client.Get(context.Background(), crdName, &v13.CustomResourceDefinition{}); err != nil {
		return err
	}

	clusters, err := clustermanager.GetRegisteredClusters(hubCluster.Client)
	if err != nil {
		return err
	}

	for _, cluster := range clusters {
		if cluster.Name == clusterName {
			return errors.Errorf("you have register a cluster named %s", clusterName)
		}
	}

	spokeRestConf, err := clientcmd.BuildConfigFromKubeconfigGetter("", func() (*clientcmdapi.Config, error) {
		return spokeConfig, nil
	})
	if err != nil {
		return errors.Wrap(err, "fail to convert spoke-cluster kubeconfig")
	}

	hubKubeToken, err := hubCluster.GenerateHubClusterKubeConfig(ctx, "")
	if err != nil {
		return errors.Wrap(err, "fail to generate the token for spoke-cluster")
	}

	spokeCluster, err := spoke.NewSpokeCluster(clusterName, spokeRestConf, hubKubeToken)
	if err != nil {
		return errors.Wrap(err, "fail to connect spoke cluster")
	}

	err = spokeCluster.InitSpokeClusterEnv(ctx)
	if err != nil {
		return errors.Wrap(err, "fail to prepare the env for spoke-cluster")
	}

	csrCheck := newTrackingSpinner("wait for managed-cluster register request ...")
	csrCheck.Start()
	defer csrCheck.Stop()
	ready, err := hubCluster.Wait4SpokeClusterReady(ctx, clusterName)
	if err != nil || !ready {
		return errors.Errorf("fail to waiting for register request")
	}

	if err = hubCluster.RegisterSpokeCluster(ctx, spokeCluster.Name); err != nil {
		return errors.Wrap(err, "fail to approve spoke cluster")
	}
	return nil
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
			if newClusterName == multicluster.ClusterLocalName {
				return fmt.Errorf("cannot use `%s` as cluster name, it is reserved as the local cluster", multicluster.ClusterLocalName)
			}
			clusterSecret, err := multicluster.GetMutableClusterSecret(context.Background(), c.Client, oldClusterName)
			if err != nil {
				return errors.Wrapf(err, "cluster %s is not mutable now", oldClusterName)
			}
			if err := clustermanager.EnsureClusterNotExists(c.Client, newClusterName); err != nil {
				return errors.Wrapf(err, "cannot set cluster name to %s", newClusterName)
			}
			if err := c.Client.Delete(context.Background(), clusterSecret); err != nil {
				return errors.Wrapf(err, "failed to rename cluster from %s to %s", oldClusterName, newClusterName)
			}
			clusterSecret.ObjectMeta = v12.ObjectMeta{
				Name:        newClusterName,
				Namespace:   multicluster.ClusterGatewaySecretNamespace,
				Labels:      clusterSecret.Labels,
				Annotations: clusterSecret.Annotations,
			}
			if err := c.Client.Create(context.Background(), clusterSecret); err != nil {
				return errors.Wrapf(err, "failed to rename cluster from %s to %s", oldClusterName, newClusterName)
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
			if clusterName == multicluster.ClusterLocalName {
				return fmt.Errorf("cannot delete `%s` cluster, it is reserved as the local cluster", multicluster.ClusterLocalName)
			}
			clusters, err := clustermanager.GetRegisteredClusters(c.Client)
			if err != nil {
				return err
			}
			var clusterType string
			for _, cluster := range clusters {
				if cluster.Name == clusterName {
					clusterType = cluster.Type
				}
			}
			if clusterType == "" {
				return errors.Errorf("cluster %s is not regitsered", clusterName)
			}

			switch clusterType {
			case string(v1alpha12.CredentialTypeX509Certificate), string(v1alpha12.CredentialTypeServiceAccountToken):
				clusterSecret, err := multicluster.GetMutableClusterSecret(context.Background(), c.Client, clusterName)
				if err != nil {
					return errors.Wrapf(err, "cluster %s is not mutable now", clusterName)
				}
				if err := c.Client.Delete(context.Background(), clusterSecret); err != nil {
					return errors.Wrapf(err, "failed to detach cluster %s", clusterName)
				}
			case "ManagedCluster":
				configPath, err := cmd.Flags().GetString(FlagKubeConfigPath)
				if err != nil {
					return errors.Wrapf(err, "failed to get cluster management type flag")
				}
				if configPath == "" {
					return errors.New("kubeconfig-path shouldn't be empty")
				}
				config, err := clientcmd.LoadFromFile(configPath)
				if err != nil {
					return err
				}
				restConfig, err := clientcmd.BuildConfigFromKubeconfigGetter("", func() (*clientcmdapi.Config, error) {
					return config, nil
				})
				if err != nil {
					return err
				}
				if err = spoke.CleanSpokeClusterEnv(restConfig); err != nil {
					return err
				}
				managedCluster := ocmclusterv1.ManagedCluster{
					ObjectMeta: v12.ObjectMeta{
						Name: clusterName,
					},
				}
				if err = c.Client.Delete(context.Background(), &managedCluster); err != nil {
					if !errors2.IsNotFound(err) {
						return err
					}
				}
			}
			cmd.Printf("Detach cluster %s successfully.\n", clusterName)
			return nil
		},
	}
	cmd.Flags().StringP(FlagKubeConfigPath, "p", "", "Specify the kubeconfig path of managed cluster. If you use ocm to manage your cluster, you must set the kubeconfig-path.")
	return cmd
}

// NewClusterProbeCommand create command to help user try health probe for existing cluster
func NewClusterProbeCommand(c *common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "probe [CLUSTER_NAME]",
		Short: "probe managed cluster",
		Args:  cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]
			if clusterName == multicluster.ClusterLocalName {
				return errors.New("you must specify a remote cluster name")
			}
			content, err := versioned.NewForConfigOrDie(c.Config).ClusterV1alpha1().ClusterGateways().RESTClient(clusterName).Get().AbsPath("healthz").DoRaw(context.TODO())
			if err != nil {
				return errors.Wrapf(err, "failed connect cluster %s", clusterName)
			}
			cmd.Printf("Connect to cluster %s successfully.\n%s\n", clusterName, string(content))
			return nil
		},
	}
	return cmd
}
