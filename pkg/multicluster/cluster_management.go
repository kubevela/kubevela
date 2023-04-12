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

package multicluster

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/kubevela/pkg/util/k8s"
	clusterv1alpha1 "github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"
	clustercommon "github.com/oam-dev/cluster-gateway/pkg/common"
	"github.com/oam-dev/cluster-register/pkg/hub"
	"github.com/oam-dev/cluster-register/pkg/spoke"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ocmclusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/policy/envbinding"
	"github.com/oam-dev/kubevela/pkg/utils"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// ContextKey defines the key in context
type ContextKey string

// KubeConfigContext marks the kubeConfig object in context
const KubeConfigContext ContextKey = "kubeConfig"

// KubeClusterConfig info for cluster management
type KubeClusterConfig struct {
	FilePath        string
	ClusterName     string
	CreateNamespace string
	*clientcmdapi.Config
	*clientcmdapi.Cluster
	*clientcmdapi.AuthInfo

	// ClusterAlreadyExistCallback callback for handling cluster already exist,
	// if no error returned, the logic will pass through
	ClusterAlreadyExistCallback func(string) bool

	// Logs records intermediate logs (which do not return error) during running
	Logs bytes.Buffer
}

// SetClusterName set cluster name if not empty
func (clusterConfig *KubeClusterConfig) SetClusterName(clusterName string) *KubeClusterConfig {
	if clusterName != "" {
		clusterConfig.ClusterName = clusterName
	}
	return clusterConfig
}

// SetCreateNamespace set create namespace, if empty, no namespace will be created
func (clusterConfig *KubeClusterConfig) SetCreateNamespace(createNamespace string) *KubeClusterConfig {
	clusterConfig.CreateNamespace = createNamespace
	return clusterConfig
}

// Validate check if config is valid for join
func (clusterConfig *KubeClusterConfig) Validate() error {
	switch clusterConfig.ClusterName {
	case "":
		return errors.Errorf("ClusterName cannot be empty")
	case ClusterLocalName:
		return errors.Errorf("ClusterName cannot be `%s`, it is reserved as the local cluster", ClusterLocalName)
	}
	return nil
}

// PostRegistration try to create namespace after cluster registered. If failed, cluster will be unregistered.
func (clusterConfig *KubeClusterConfig) PostRegistration(ctx context.Context, cli client.Client) error {
	if clusterConfig.CreateNamespace == "" {
		return nil
	}
	// retry 3 times.
	for i := 0; i < 3; i++ {
		if err := ensureNamespaceExists(ctx, cli, clusterConfig.ClusterName, clusterConfig.CreateNamespace); err != nil {
			// Cluster gateway discovers the cluster maybe be deferred, so we should retry.
			if strings.Contains(err.Error(), "no such cluster") {
				if i < 2 {
					time.Sleep(time.Second * 1)
					continue
				}
			}
			_ = DetachCluster(ctx, cli, clusterConfig.ClusterName, DetachClusterManagedClusterKubeConfigPathOption(clusterConfig.FilePath))
			return fmt.Errorf("failed to ensure %s namespace installed in cluster %s: %w", clusterConfig.CreateNamespace, clusterConfig.ClusterName, err)
		}
		break
	}
	return nil
}

func (clusterConfig *KubeClusterConfig) createOrUpdateClusterSecret(ctx context.Context, cli client.Client, withEndpoint bool) error {
	var credentialType clusterv1alpha1.CredentialType
	data := map[string][]byte{}
	if withEndpoint {
		data["endpoint"] = []byte(clusterConfig.Cluster.Server)
		if !clusterConfig.Cluster.InsecureSkipTLSVerify {
			data["ca.crt"] = clusterConfig.Cluster.CertificateAuthorityData
		}
	}
	if len(clusterConfig.AuthInfo.Token) > 0 {
		credentialType = clusterv1alpha1.CredentialTypeServiceAccountToken
		data["token"] = []byte(clusterConfig.AuthInfo.Token)
	} else {
		credentialType = clusterv1alpha1.CredentialTypeX509Certificate
		data["tls.crt"] = clusterConfig.AuthInfo.ClientCertificateData
		data["tls.key"] = clusterConfig.AuthInfo.ClientKeyData
	}
	secret := &corev1.Secret{}
	if err := cli.Get(ctx, apitypes.NamespacedName{Name: clusterConfig.ClusterName, Namespace: ClusterGatewaySecretNamespace}, secret); client.IgnoreNotFound(err) != nil {
		return err
	}
	secret.Name = clusterConfig.ClusterName
	secret.Namespace = ClusterGatewaySecretNamespace
	secret.Type = corev1.SecretTypeOpaque
	_ = k8s.AddLabel(secret, clustercommon.LabelKeyClusterCredentialType, string(credentialType))
	secret.Data = data
	if secret.ResourceVersion == "" {
		return cli.Create(ctx, secret)
	}
	return cli.Update(ctx, secret)
}

// RegisterByVelaSecret create cluster secrets for KubeVela to use
func (clusterConfig *KubeClusterConfig) RegisterByVelaSecret(ctx context.Context, cli client.Client) error {
	cluster, err := NewClusterClient(cli).Get(ctx, clusterConfig.ClusterName)
	if client.IgnoreNotFound(err) != nil {
		return err
	}
	if cluster != nil {
		if clusterConfig.ClusterAlreadyExistCallback == nil {
			return fmt.Errorf("cluster %s already exists", cluster.Name)
		}
		if !clusterConfig.ClusterAlreadyExistCallback(clusterConfig.ClusterName) {
			return nil
		}
		if cluster.Spec.CredentialType == clusterv1alpha1.CredentialTypeInternal || cluster.Spec.CredentialType == clusterv1alpha1.CredentialTypeOCMManagedCluster {
			return fmt.Errorf("cannot override %s typed cluster", cluster.Spec.CredentialType)
		}
	}

	if err := clusterConfig.createOrUpdateClusterSecret(ctx, cli, true); err != nil {
		return errors.Wrapf(err, "failed to add cluster to kubernetes")
	}
	return clusterConfig.PostRegistration(ctx, cli)
}

// CreateBootstrapConfigMapIfNotExists alternative to
// https://github.com/kubernetes/kubernetes/blob/v1.24.1/cmd/kubeadm/app/phases/bootstraptoken/clusterinfo/clusterinfo.go#L43
func CreateBootstrapConfigMapIfNotExists(ctx context.Context, cli client.Client) error {
	cm := &corev1.ConfigMap{}
	key := apitypes.NamespacedName{Namespace: metav1.NamespacePublic, Name: "cluster-info"}
	if err := cli.Get(ctx, key, cm); err != nil {
		if apierrors.IsNotFound(err) {
			cm.ObjectMeta = metav1.ObjectMeta{Namespace: key.Namespace, Name: key.Name}
			adminConfig, err := clientcmd.NewDefaultPathOptions().GetStartingConfig()
			if err != nil {
				return err
			}
			adminCluster := adminConfig.Contexts[adminConfig.CurrentContext].Cluster
			bs, err := clientcmd.Write(clientcmdapi.Config{
				Clusters: map[string]*clientcmdapi.Cluster{"": adminConfig.Clusters[adminCluster]},
			})
			if err != nil {
				return err
			}
			cm.Data = map[string]string{"kubeconfig": string(bs)}
			return cli.Create(ctx, cm)
		}
		return err
	}
	return nil
}

// RegisterClusterManagedByOCM create ocm managed cluster for use
// TODO(somefive): OCM ManagedCluster only support cli join now
func (clusterConfig *KubeClusterConfig) RegisterClusterManagedByOCM(ctx context.Context, cli client.Client, args *JoinClusterArgs) error {
	newTrackingSpinner := args.trackingSpinnerFactory
	hubCluster, err := hub.NewHubCluster(args.hubConfig)
	if err != nil {
		return errors.Wrap(err, "fail to create client connect to hub cluster")
	}

	hubTracker := newTrackingSpinner("Checking the environment of hub cluster..")
	hubTracker.FinalMSG = "Hub cluster all set, continue registration.\n"
	hubTracker.Start()
	crdName := apitypes.NamespacedName{Name: "managedclusters." + ocmclusterv1.GroupName}
	if err := hubCluster.Client.Get(context.Background(), crdName, &apiextensionsv1.CustomResourceDefinition{}); err != nil {
		return err
	}

	clusters, err := ListVirtualClusters(context.Background(), hubCluster.Client)
	if err != nil {
		return err
	}

	for _, cluster := range clusters {
		if cluster.Name == clusterConfig.ClusterName && cluster.Accepted {
			return errors.Errorf("you have register a cluster named %s", clusterConfig.ClusterName)
		}
	}
	hubTracker.Stop()

	spokeRestConf, err := clientcmd.BuildConfigFromKubeconfigGetter("", func() (*clientcmdapi.Config, error) {
		return clusterConfig.Config, nil
	})
	if err != nil {
		return errors.Wrap(err, "fail to convert spoke-cluster kubeconfig")
	}

	if err = CreateBootstrapConfigMapIfNotExists(ctx, cli); err != nil {
		return fmt.Errorf("failed to ensure cluster-info ConfigMap in kube-public namespace exists: %w", err)
	}
	spokeTracker := newTrackingSpinner("Building registration config for the managed cluster")
	spokeTracker.FinalMSG = "Successfully prepared registration config.\n"
	spokeTracker.Start()
	overridingRegistrationEndpoint := ""
	if !*args.inClusterBootstrap {
		args.ioStreams.Infof("Using the api endpoint from hub kubeconfig %q as registration entry.\n", args.hubConfig.Host)
		overridingRegistrationEndpoint = args.hubConfig.Host
	}

	hubKubeToken, err := hubCluster.GenerateHubClusterKubeConfig(ctx, overridingRegistrationEndpoint)
	if err != nil {
		return errors.Wrap(err, "fail to generate the token for spoke-cluster")
	}

	spokeCluster, err := spoke.NewSpokeCluster(clusterConfig.ClusterName, spokeRestConf, hubKubeToken)
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

	args.ioStreams.Infof("Approving the CSR for cluster %q.\n", spokeCluster.Name)
	if err := hubCluster.ApproveCSR(ctx, spokeCluster.Name); err != nil {
		return errors.Wrap(err, "failed found CSR created by registration agent")
	}

	ready, err := hubCluster.WaitForSpokeClusterReady(ctx, clusterConfig.ClusterName)
	if err != nil || !ready {
		return errors.Errorf("fail to waiting for register request")
	}

	if err = hubCluster.RegisterSpokeCluster(ctx, spokeCluster.Name); err != nil {
		return errors.Wrap(err, "fail to approve spoke cluster")
	}
	return nil
}

// LoadKubeClusterConfigFromFile create KubeClusterConfig from kubeconfig file
func LoadKubeClusterConfigFromFile(filepath string) (*KubeClusterConfig, error) {
	clusterConfig := &KubeClusterConfig{FilePath: filepath}
	var err error
	clusterConfig.Config, err = clientcmd.LoadFromFile(filepath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get kubeconfig")
	}
	if len(clusterConfig.Config.CurrentContext) == 0 {
		return nil, fmt.Errorf("current-context is not set")
	}
	var ok bool
	ctx, ok := clusterConfig.Config.Contexts[clusterConfig.Config.CurrentContext]
	if !ok {
		return nil, fmt.Errorf("current-context %s not found", clusterConfig.Config.CurrentContext)
	}
	clusterConfig.Cluster, ok = clusterConfig.Config.Clusters[ctx.Cluster]
	if !ok {
		return nil, fmt.Errorf("cluster %s not found", ctx.Cluster)
	}
	clusterConfig.AuthInfo, ok = clusterConfig.Config.AuthInfos[ctx.AuthInfo]
	if !ok {
		return nil, fmt.Errorf("authInfo %s not found", ctx.AuthInfo)
	}
	clusterConfig.ClusterName = ctx.Cluster
	if endpoint, err := utils.ParseAPIServerEndpoint(clusterConfig.Cluster.Server); err == nil {
		clusterConfig.Cluster.Server = endpoint
	} else {
		_, _ = fmt.Fprintf(&clusterConfig.Logs, "failed to parse server endpoint: %v", err)
	}
	return clusterConfig, nil
}

const (
	// ClusterGateWayEngine cluster-gateway cluster management solution
	ClusterGateWayEngine = "cluster-gateway"
	// OCMEngine ocm cluster management solution
	OCMEngine = "ocm"
)

// JoinClusterArgs args for join cluster
type JoinClusterArgs struct {
	engine                      string
	createNamespace             string
	ioStreams                   cmdutil.IOStreams
	hubConfig                   *rest.Config
	inClusterBootstrap          *bool
	trackingSpinnerFactory      func(string) *spinner.Spinner
	clusterAlreadyExistCallback func(string) bool
}

func newJoinClusterArgs(options ...JoinClusterOption) *JoinClusterArgs {
	args := &JoinClusterArgs{
		engine: ClusterGateWayEngine,
	}
	for _, op := range options {
		op.ApplyToArgs(args)
	}
	return args
}

// JoinClusterOption option for join cluster
type JoinClusterOption interface {
	ApplyToArgs(args *JoinClusterArgs)
}

// JoinClusterCreateNamespaceOption create namespace when join cluster, if empty, no creation
type JoinClusterCreateNamespaceOption string

// ApplyToArgs apply to args
func (op JoinClusterCreateNamespaceOption) ApplyToArgs(args *JoinClusterArgs) {
	args.createNamespace = string(op)
}

// JoinClusterEngineOption configure engine for join cluster, either cluster-gateway or ocm
type JoinClusterEngineOption string

// ApplyToArgs apply to args
func (op JoinClusterEngineOption) ApplyToArgs(args *JoinClusterArgs) {
	args.engine = string(op)
}

// JoinClusterAlreadyExistCallback configure the callback when cluster already exist
type JoinClusterAlreadyExistCallback func(string) bool

// ApplyToArgs apply to args
func (op JoinClusterAlreadyExistCallback) ApplyToArgs(args *JoinClusterArgs) {
	args.clusterAlreadyExistCallback = op
}

// JoinClusterOCMOptions options used when joining clusters by ocm, only support cli for now
type JoinClusterOCMOptions struct {
	IoStreams              cmdutil.IOStreams
	HubConfig              *rest.Config
	InClusterBootstrap     *bool
	TrackingSpinnerFactory func(string) *spinner.Spinner
}

// ApplyToArgs apply to args
func (op JoinClusterOCMOptions) ApplyToArgs(args *JoinClusterArgs) {
	args.ioStreams = op.IoStreams
	args.hubConfig = op.HubConfig
	args.inClusterBootstrap = op.InClusterBootstrap
	args.trackingSpinnerFactory = op.TrackingSpinnerFactory
}

// JoinClusterByKubeConfig add child cluster by kubeconfig path, return cluster info and error
func JoinClusterByKubeConfig(ctx context.Context, cli client.Client, kubeconfigPath string, clusterName string, options ...JoinClusterOption) (*KubeClusterConfig, error) {
	args := newJoinClusterArgs(options...)
	clusterConfig, err := LoadKubeClusterConfigFromFile(kubeconfigPath)
	if err != nil {
		return nil, err
	}
	if err := clusterConfig.SetClusterName(clusterName).SetCreateNamespace(args.createNamespace).Validate(); err != nil {
		return nil, err
	}
	clusterConfig.ClusterAlreadyExistCallback = args.clusterAlreadyExistCallback
	switch args.engine {
	case ClusterGateWayEngine:
		if err = clusterConfig.RegisterByVelaSecret(ctx, cli); err != nil {
			return nil, err
		}
	case OCMEngine:
		if args.inClusterBootstrap == nil {
			return nil, errors.Wrapf(err, "failed to determine the registration endpoint for the hub cluster "+
				"when parsing --in-cluster-bootstrap flag")
		}
		if err = clusterConfig.RegisterClusterManagedByOCM(ctx, cli, args); err != nil {
			return clusterConfig, err
		}
	}
	if cfg, ok := ctx.Value(KubeConfigContext).(*rest.Config); ok {
		if err = SetClusterVersionInfo(ctx, cfg, clusterConfig.ClusterName); err != nil {
			return nil, err
		}
	}
	return clusterConfig, nil
}

// DetachClusterArgs args for detaching cluster
type DetachClusterArgs struct {
	managedClusterKubeConfigPath string
}

func newDetachClusterArgs(options ...DetachClusterOption) *DetachClusterArgs {
	args := &DetachClusterArgs{}
	for _, op := range options {
		op.ApplyToArgs(args)
	}
	return args
}

// DetachClusterOption option for detach cluster
type DetachClusterOption interface {
	ApplyToArgs(args *DetachClusterArgs)
}

// DetachClusterManagedClusterKubeConfigPathOption configure the managed cluster kubeconfig path while detach ocm cluster
type DetachClusterManagedClusterKubeConfigPathOption string

// ApplyToArgs apply to args
func (op DetachClusterManagedClusterKubeConfigPathOption) ApplyToArgs(args *DetachClusterArgs) {
	args.managedClusterKubeConfigPath = string(op)
}

// DetachCluster detach cluster by name, if cluster is using by application, it will return error
func DetachCluster(ctx context.Context, cli client.Client, clusterName string, options ...DetachClusterOption) error {
	args := newDetachClusterArgs(options...)
	if clusterName == ClusterLocalName {
		return ErrReservedLocalClusterName
	}
	vc, err := NewClusterClient(cli).Get(ctx, clusterName)
	if err != nil {
		return err
	}

	switch vc.Spec.CredentialType {
	case clusterv1alpha1.CredentialTypeX509Certificate, clusterv1alpha1.CredentialTypeServiceAccountToken:
		clusterSecret, err := getMutableClusterSecret(ctx, cli, clusterName)
		if err != nil {
			return errors.Wrapf(err, "cluster %s is not mutable now", clusterName)
		}
		if err := cli.Delete(ctx, clusterSecret); err != nil {
			return errors.Wrapf(err, "failed to detach cluster %s", clusterName)
		}
	case clusterv1alpha1.CredentialTypeOCMManagedCluster:
		if args.managedClusterKubeConfigPath == "" {
			return errors.New("kubeconfig-path must be set to detach ocm managed cluster")
		}
		config, err := clientcmd.LoadFromFile(args.managedClusterKubeConfigPath)
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
		managedCluster := ocmclusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: clusterName}}
		if err = cli.Delete(ctx, &managedCluster); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
		}
	case clusterv1alpha1.CredentialTypeInternal:
		return fmt.Errorf("cannot detach internal cluster `local`")
	}
	return nil
}

// RenameCluster rename cluster
func RenameCluster(ctx context.Context, k8sClient client.Client, oldClusterName string, newClusterName string) error {
	if newClusterName == ClusterLocalName {
		return ErrReservedLocalClusterName
	}
	clusterSecret, err := getMutableClusterSecret(ctx, k8sClient, oldClusterName)
	if err != nil {
		return errors.Wrapf(err, "cluster %s is not mutable now", oldClusterName)
	}
	if err := ensureClusterNotExists(ctx, k8sClient, newClusterName); err != nil {
		return errors.Wrapf(err, "cannot set cluster name to %s", newClusterName)
	}
	if err := k8sClient.Delete(ctx, clusterSecret); err != nil {
		return errors.Wrapf(err, "failed to rename cluster from %s to %s", oldClusterName, newClusterName)
	}
	clusterSecret.ObjectMeta = metav1.ObjectMeta{
		Name:        newClusterName,
		Namespace:   ClusterGatewaySecretNamespace,
		Labels:      clusterSecret.Labels,
		Annotations: clusterSecret.Annotations,
	}
	if err := k8sClient.Create(ctx, clusterSecret); err != nil {
		return errors.Wrapf(err, "failed to rename cluster from %s to %s", oldClusterName, newClusterName)
	}
	return nil
}

// AliasCluster alias cluster
func AliasCluster(ctx context.Context, cli client.Client, clusterName string, aliasName string) error {
	if clusterName == ClusterLocalName {
		return ErrReservedLocalClusterName
	}
	vc, err := GetVirtualCluster(ctx, cli, clusterName)
	if err != nil {
		return err
	}
	setClusterAlias(vc.Object, aliasName)
	return cli.Update(ctx, vc.Object)
}

// ensureClusterNotExists will check the cluster is not existed in control plane
func ensureClusterNotExists(ctx context.Context, c client.Client, clusterName string) error {
	_, err := NewClusterClient(c).Get(ctx, clusterName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	return ErrClusterExists
}

// ensureNamespaceExists ensures vela namespace  to be installed in child cluster
func ensureNamespaceExists(ctx context.Context, c client.Client, clusterName string, createNamespace string) error {
	remoteCtx := ContextWithClusterName(ctx, clusterName)
	if err := c.Get(remoteCtx, apitypes.NamespacedName{Name: createNamespace}, &corev1.Namespace{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to check if namespace %s exists", createNamespace)
		}
		if err = c.Create(remoteCtx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: createNamespace}}); err != nil {
			return errors.Wrapf(err, "failed to create namespace %s", createNamespace)
		}
	}
	return nil
}

// getMutableClusterSecret retrieves the cluster secret and check if any application is using the cluster
// TODO(somefive): should rework the logic of checking application cluster usage
func getMutableClusterSecret(ctx context.Context, c client.Client, clusterName string) (*corev1.Secret, error) {
	clusterSecret := &corev1.Secret{}
	if err := c.Get(ctx, apitypes.NamespacedName{Namespace: ClusterGatewaySecretNamespace, Name: clusterName}, clusterSecret); err != nil {
		return nil, errors.Wrapf(err, "failed to find target cluster secret %s", clusterName)
	}
	labels := clusterSecret.GetLabels()
	if labels == nil || labels[clustercommon.LabelKeyClusterCredentialType] == "" {
		return nil, fmt.Errorf("invalid cluster secret %s: cluster credential type label %s is not set", clusterName, clustercommon.LabelKeyClusterCredentialType)
	}
	apps := &v1beta1.ApplicationList{}
	if err := c.List(ctx, apps); err != nil {
		return nil, errors.Wrap(err, "failed to find applications to check clusters")
	}
	errs := velaerrors.ErrorList{}
	for _, app := range apps.Items {
		status, err := envbinding.GetEnvBindingPolicyStatus(app.DeepCopy(), "")
		if err == nil && status != nil {
			for _, env := range status.Envs {
				for _, placement := range env.Placements {
					if placement.Cluster == clusterName {
						errs = append(errs, fmt.Errorf("application %s/%s (env: %s) is currently using cluster %s", app.Namespace, app.Name, env.Env, clusterName))
					}
				}
			}
		}
	}
	if errs.HasError() {
		return nil, errors.Wrapf(errs, "cluster %s is in use now", clusterName)
	}
	return clusterSecret, nil
}
