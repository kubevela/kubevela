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
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ocmclusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1alpha1 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	"github.com/oam-dev/cluster-gateway/pkg/generated/clientset/versioned"
	"github.com/oam-dev/cluster-register/pkg/hub"
	"github.com/oam-dev/cluster-register/pkg/spoke"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/clustermanager"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils"
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

	// ClusterGateWayClusterManagement cluster-gateway cluster management solution
	ClusterGateWayClusterManagement = "cluster-gateway"
	// OCMClusterManagement ocm cluster management solution
	OCMClusterManagement = "ocm"
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
			table := newUITable().AddRow("CLUSTER", "TYPE", "ENDPOINT")
			client, err := c.GetClient()
			if err != nil {
				return err
			}
			clusters, err := clustermanager.GetRegisteredClusters(client)
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

func ensureVelaSystemNamespaceInstalled(c client.Client, clusterName string, createNamespace string) error {
	ctx := context.Background()
	remoteCtx := multicluster.ContextWithClusterName(ctx, clusterName)
	if err := c.Get(remoteCtx, k8stypes.NamespacedName{Name: createNamespace}, &corev1.Namespace{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to check vela-system ")
		}
		if err = c.Create(remoteCtx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: createNamespace}}); err != nil {
			return errors.Wrapf(err, "failed to create vela-system namespace")
		}
	}
	return nil
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

			// get need created namespace in managed cluster
			createNamespace, err := cmd.Flags().GetString(CreateNamespace)
			if err != nil {
				return errors.Wrapf(err, "failed to get create namespace")
			}

			if createNamespace == "" {
				createNamespace = types.DefaultKubeVelaNS
			}
			client, err := c.GetClient()
			if err != nil {
				return err
			}
			restConfig, err := c.GetConfig()
			if err != nil {
				return err
			}
			switch clusterManagementType {
			case ClusterGateWayClusterManagement:
				if endpoint, err := utils.ParseAPIServerEndpoint(cluster.Server); err == nil {
					cluster.Server = endpoint
				} else {
					ioStreams.Infof("failed to parse server endpoint: %v", err)
				}
				if err = registerClusterManagedByVela(client, cluster, authInfo, clusterName, createNamespace); err != nil {
					return err
				}
			case OCMClusterManagement:
				inClusterBootstrap, err := cmd.Flags().GetBool(FlagInClusterBootstrap)
				if err != nil {
					return errors.Wrapf(err, "failed to determine the registration endpoint for the hub cluster "+
						"when parsing --in-cluster-bootstrap flag")
				}
				if err = registerClusterManagedByOCM(ioStreams, restConfig, config, clusterName, inClusterBootstrap); err != nil {
					return err
				}
			}
			cmd.Printf("Successfully add cluster %s, endpoint: %s.\n", clusterName, cluster.Server)
			return nil
		},
	}
	cmd.Flags().StringP(FlagClusterName, "n", "", "Specify the cluster name. If empty, it will use the cluster name in config file. Default to be empty.")
	cmd.Flags().StringP(FlagClusterManagementEngine, "t", "", "Specify the cluster management engine. If empty, it will use cluster-gateway cluster management solution. Default to be empty.")
	cmd.Flags().StringP(CreateNamespace, "", "", "Specifies the namespace need to create in managedCluster")
	cmd.Flags().BoolP(FlagInClusterBootstrap, "", true, "If true, the registering managed cluster "+
		`will use the internal endpoint prescribed in the hub cluster's configmap "kube-public/cluster-info to register "`+
		"itself to the hub cluster. Otherwise use the original endpoint from the hub kubeconfig.")
	return cmd
}

func registerClusterManagedByVela(k8sClient client.Client, cluster *clientcmdapi.Cluster, authInfo *clientcmdapi.AuthInfo, clusterName string, createNamespace string) error {
	if err := clustermanager.EnsureClusterNotExists(k8sClient, clusterName); err != nil {
		return errors.Wrapf(err, "cannot use cluster name %s", clusterName)
	}
	var credentialType clusterv1alpha1.CredentialType
	data := map[string][]byte{
		"endpoint": []byte(cluster.Server),
		"ca.crt":   cluster.CertificateAuthorityData,
	}
	if len(authInfo.Token) > 0 {
		credentialType = clusterv1alpha1.CredentialTypeServiceAccountToken
		data["token"] = []byte(authInfo.Token)
	} else {
		credentialType = clusterv1alpha1.CredentialTypeX509Certificate
		data["tls.crt"] = authInfo.ClientCertificateData
		data["tls.key"] = authInfo.ClientKeyData
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: multicluster.ClusterGatewaySecretNamespace,
			Labels: map[string]string{
				clusterv1alpha1.LabelKeyClusterCredentialType: string(credentialType),
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
	if err := k8sClient.Create(context.Background(), secret); err != nil {
		return errors.Wrapf(err, "failed to add cluster to kubernetes")
	}
	if err := ensureVelaSystemNamespaceInstalled(k8sClient, clusterName, createNamespace); err != nil {
		_ = k8sClient.Delete(context.Background(), secret)
		return errors.Wrapf(err, "failed to ensure vela-system namespace installed in cluster %s", clusterName)
	}
	return nil
}

func registerClusterManagedByOCM(ioStreams cmdutil.IOStreams, hubConfig *rest.Config, spokeConfig *clientcmdapi.Config, clusterName string, inClusterBootstrap bool) error {
	ctx := context.Background()
	hubCluster, err := hub.NewHubCluster(hubConfig)
	if err != nil {
		return errors.Wrap(err, "fail to create client connect to hub cluster")
	}

	hubTracker := newTrackingSpinner("Checking the environment of hub cluster..")
	hubTracker.FinalMSG = "Hub cluster all set, continue registration.\n"
	hubTracker.Start()
	crdName := k8stypes.NamespacedName{Name: "managedclusters." + ocmclusterv1.GroupName}
	if err := hubCluster.Client.Get(context.Background(), crdName, &apiextensionsv1.CustomResourceDefinition{}); err != nil {
		return err
	}

	clusters, err := clustermanager.GetRegisteredClusters(hubCluster.Client)
	if err != nil {
		return err
	}

	for _, cluster := range clusters {
		if cluster.Name == clusterName && cluster.Accepted {
			return errors.Errorf("you have register a cluster named %s", clusterName)
		}
	}
	hubTracker.Stop()

	spokeRestConf, err := clientcmd.BuildConfigFromKubeconfigGetter("", func() (*clientcmdapi.Config, error) {
		return spokeConfig, nil
	})
	if err != nil {
		return errors.Wrap(err, "fail to convert spoke-cluster kubeconfig")
	}

	spokeTracker := newTrackingSpinner("Building registration config for the managed cluster")
	spokeTracker.FinalMSG = "Successfully prepared registration config.\n"
	spokeTracker.Start()
	overridingRegistrationEndpoint := ""
	if !inClusterBootstrap {
		ioStreams.Infof("Using the api endpoint from hub kubeconfig %q as registration entry.\n", hubConfig.Host)
		overridingRegistrationEndpoint = hubConfig.Host
	}
	hubKubeToken, err := hubCluster.GenerateHubClusterKubeConfig(ctx, overridingRegistrationEndpoint)
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
	spokeTracker.Stop()

	registrationOperatorTracker := newTrackingSpinner("Waiting for registration operators running: (`kubectl -n open-cluster-management get pod -l app=klusterlet`)")
	registrationOperatorTracker.FinalMSG = "Registration operator successfully deployed.\n"
	registrationOperatorTracker.Start()
	if err := spokeCluster.WaitForRegistrationOperatorReady(ctx); err != nil {
		return errors.Wrap(err, "fail to setup registration operator for spoke-cluster")
	}
	registrationOperatorTracker.Stop()

	registrationAgentTracker := newTrackingSpinner("Waiting for registration agent running: (`kubectl -n open-cluster-management-agent get pod -l app=klusterlet-registration-agent`)")
	registrationAgentTracker.FinalMSG = "Registration agent successfully deployed.\n"
	registrationAgentTracker.Start()
	if err := spokeCluster.WaitForRegistrationAgentReady(ctx); err != nil {
		return errors.Wrap(err, "fail to setup registration agent for spoke-cluster")
	}
	registrationAgentTracker.Stop()

	csrCreationTracker := newTrackingSpinner("Waiting for CSRs created (`kubectl get csr -l open-cluster-management.io/cluster-name=" + spokeCluster.Name + "`)")
	csrCreationTracker.FinalMSG = "Successfully found corresponding CSR from the agent.\n"
	csrCreationTracker.Start()
	if err := hubCluster.WaitForCSRCreated(ctx, spokeCluster.Name); err != nil {
		return errors.Wrap(err, "failed found CSR created by registration agent")
	}
	csrCreationTracker.Stop()

	ioStreams.Infof("Approving the CSR for cluster %q.\n", spokeCluster.Name)
	if err := hubCluster.ApproveCSR(ctx, spokeCluster.Name); err != nil {
		return errors.Wrap(err, "failed found CSR created by registration agent")
	}

	ready, err := hubCluster.WaitForSpokeClusterReady(ctx, clusterName)
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
		Short: "rename managed cluster.",
		Long:  "rename managed cluster.",
		Args:  cobra.ExactValidArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			oldClusterName := args[0]
			newClusterName := args[1]
			if newClusterName == multicluster.ClusterLocalName {
				return fmt.Errorf("cannot use `%s` as cluster name, it is reserved as the local cluster", multicluster.ClusterLocalName)
			}
			client, err := c.GetClient()
			if err != nil {
				return err
			}
			clusterSecret, err := multicluster.GetMutableClusterSecret(context.Background(), client, oldClusterName)
			if err != nil {
				return errors.Wrapf(err, "cluster %s is not mutable now", oldClusterName)
			}
			if err := clustermanager.EnsureClusterNotExists(client, newClusterName); err != nil {
				return errors.Wrapf(err, "cannot set cluster name to %s", newClusterName)
			}
			if err := client.Delete(context.Background(), clusterSecret); err != nil {
				return errors.Wrapf(err, "failed to rename cluster from %s to %s", oldClusterName, newClusterName)
			}
			clusterSecret.ObjectMeta = metav1.ObjectMeta{
				Name:        newClusterName,
				Namespace:   multicluster.ClusterGatewaySecretNamespace,
				Labels:      clusterSecret.Labels,
				Annotations: clusterSecret.Annotations,
			}
			if err := client.Create(context.Background(), clusterSecret); err != nil {
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
		Short: "detach managed cluster.",
		Long:  "detach managed cluster.",
		Args:  cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterName := args[0]
			if clusterName == multicluster.ClusterLocalName {
				return fmt.Errorf("cannot delete `%s` cluster, it is reserved as the local cluster", multicluster.ClusterLocalName)
			}
			client, err := c.GetClient()
			if err != nil {
				return err
			}
			clusters, err := clustermanager.GetRegisteredClusters(client)
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
			case string(clusterv1alpha1.CredentialTypeX509Certificate), string(clusterv1alpha1.CredentialTypeServiceAccountToken):
				clusterSecret, err := multicluster.GetMutableClusterSecret(context.Background(), client, clusterName)
				if err != nil {
					return errors.Wrapf(err, "cluster %s is not mutable now", clusterName)
				}
				if err := client.Delete(context.Background(), clusterSecret); err != nil {
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
					ObjectMeta: metav1.ObjectMeta{
						Name: clusterName,
					},
				}
				if err = client.Delete(context.Background(), &managedCluster); err != nil {
					if !apierrors.IsNotFound(err) {
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
