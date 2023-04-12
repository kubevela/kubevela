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

package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/auth"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	cmdutil "github.com/oam-dev/kubevela/pkg/cmd/util"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

// AuthCommandGroup commands for create resources or configuration
func AuthCommandGroup(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: i18n.T("Manage identity and authorizations."),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
	}
	cmd.AddCommand(NewGenKubeConfigCommand(f, streams))
	cmd.AddCommand(NewListPrivilegesCommand(f, streams))
	cmd.AddCommand(NewGrantPrivilegesCommand(f, streams))
	return cmd
}

// GenKubeConfigOptions options for create kubeconfig
type GenKubeConfigOptions struct {
	auth.Identity
	util.IOStreams
}

// Complete .
func (opt *GenKubeConfigOptions) Complete(f velacmd.Factory, cmd *cobra.Command) {
	if opt.Identity.ServiceAccount != "" {
		opt.Identity.ServiceAccountNamespace = velacmd.GetNamespace(f, cmd)
	}
	opt.Regularize()
}

// Validate .
func (opt *GenKubeConfigOptions) Validate() error {
	return opt.Identity.Validate()
}

// Run .
func (opt *GenKubeConfigOptions) Run(f velacmd.Factory) error {
	ctx := context.Background()
	cli, err := kubernetes.NewForConfig(f.Config())
	if err != nil {
		return err
	}
	cfg, err := clientcmd.NewDefaultPathOptions().GetStartingConfig()
	if err != nil {
		return err
	}
	cfg, err = auth.GenerateKubeConfig(ctx, cli, cfg, opt.IOStreams.ErrOut, auth.KubeConfigWithIdentityGenerateOption(opt.Identity))
	if err != nil {
		return err
	}
	bs, err := clientcmd.Write(*cfg)
	if err != nil {
		return err
	}
	_, err = opt.Out.Write(bs)
	return err
}

var (
	genKubeConfigLong = templates.LongDesc(i18n.T(`
		Generate kubeconfig for user

		Generate a new kubeconfig with specified identity. By default, the generated kubeconfig 
		will reuse the certificate-authority-data in the cluster config from the current used 
		kubeconfig. All contexts, clusters and users that are not in use will not be included
		in the generated kubeconfig.

		To generate a new kubeconfig for given user and groups, use the --user and --group flag.
		Multiple --group flags is allowed. The group kubevela:client is added to the groups by 
		default. The identity in the current kubeconfig should be able to approve 
		CertificateSigningRequest in the kubernetes cluster. See
		https://kubernetes.io/docs/reference/access-authn-authz/certificate-signing-requests/
		for details.

		To generate a kubeconfig based on existing ServiceAccount in your cluster, use the 
		--serviceaccount flag. The corresponding secret token and ca data will be embedded in 
		the generated kubeconfig, which allows you to act as the serviceaccount.`))

	generateKubeConfigExample = templates.Examples(i18n.T(`
		# Generate a kubeconfig with provided user
		vela auth gen-kubeconfig --user new-user
		
		# Generate a kubeconfig with provided user and group
		vela auth gen-kubeconfig --user new-user --group kubevela:developer
		
		# Generate a kubeconfig with provided user and groups
		vela auth gen-kubeconfig --user new-user --group kubevela:developer --group my-org:my-team

		# Generate a kubeconfig with provided serviceaccount
		vela auth gen-kubeconfig --serviceaccount default -n demo`))
)

// NewGenKubeConfigCommand generate kubeconfig for given user and groups
func NewGenKubeConfigCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	o := &GenKubeConfigOptions{IOStreams: streams}
	cmd := &cobra.Command{
		Use:                   "gen-kubeconfig",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Generate kubeconfig for user"),
		Long:                  genKubeConfigLong,
		Example:               generateKubeConfigExample,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			o.Complete(f, cmd)
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run(f))
		},
	}
	cmd.Flags().StringVarP(&o.User, "user", "u", o.User, "The user of the generated kubeconfig. If set, an X509-based kubeconfig will be intended to create. It will be embedded as the Subject in the X509 certificate.")
	cmd.Flags().StringSliceVarP(&o.Groups, "group", "g", o.Groups, "The groups of the generated kubeconfig. This flag only works when `--user` is set. It will be embedded as the Organization in the X509 certificate.")
	cmd.Flags().StringVarP(&o.ServiceAccount, "serviceaccount", "", o.ServiceAccount, "The serviceaccount of the generated kubeconfig. If set, a kubeconfig will be generated based on the secret token of the serviceaccount. Cannot be set when `--user` presents.")
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"serviceaccount", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if strings.TrimSpace(o.User) != "" {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			namespace := velacmd.GetNamespace(f, cmd)
			return velacmd.GetServiceAccountForCompletion(cmd.Context(), f, namespace, toComplete)
		}))

	return velacmd.NewCommandBuilder(f, cmd).
		WithNamespaceFlag(velacmd.UsageOption("The namespace of the serviceaccount. This flag only works when `--serviceaccount` is set.")).
		WithStreams(streams).
		WithResponsiveWriter().
		Build()
}

// ListPrivilegesOptions options for list privileges
type ListPrivilegesOptions struct {
	auth.Identity
	KubeConfig string
	Clusters   []string
	util.IOStreams
}

// Complete .
func (opt *ListPrivilegesOptions) Complete(f velacmd.Factory, cmd *cobra.Command) {
	if opt.KubeConfig != "" {
		identity, err := auth.ReadIdentityFromKubeConfig(opt.KubeConfig)
		cmdutil.CheckErr(err)
		opt.Identity = *identity
	}
	if opt.Identity.ServiceAccount != "" {
		opt.Identity.ServiceAccountNamespace = velacmd.GetNamespace(f, cmd)
	}
	opt.Clusters = velacmd.GetClusters(cmd)
	opt.Regularize()
}

// Validate .
func (opt *ListPrivilegesOptions) Validate(f velacmd.Factory, cmd *cobra.Command) error {
	if err := opt.Identity.Validate(); err != nil {
		return err
	}
	for _, cluster := range opt.Clusters {
		if _, err := multicluster.NewClusterClient(f.Client()).Get(cmd.Context(), cluster); err != nil {
			return fmt.Errorf("failed to find cluster %s: %w", cluster, err)
		}
	}
	return nil
}

// Run .
func (opt *ListPrivilegesOptions) Run(f velacmd.Factory, cmd *cobra.Command) error {
	ctx := cmd.Context()
	m, err := auth.ListPrivileges(ctx, f.Client(), opt.Clusters, &opt.Identity)
	if err != nil {
		return err
	}
	width, _, err := term.GetSize(0)
	if err != nil {
		width = 80
	}
	_, _ = opt.Out.Write([]byte(auth.PrettyPrintPrivileges(&opt.Identity, m, opt.Clusters, uint(width)-40)))
	return nil
}

var (
	listPrivilegesLong = templates.LongDesc(i18n.T(`
		List privileges for user

		List privileges that user has in clusters. Use --user/--group to check the privileges
		for specified user and group. They can be jointly configured to see the union of
		privileges. Use --serviceaccount and -n/--namespace to see the privileges for 
		ServiceAccount. You can also use --kubeconfig to use the identity inside implicitly. 
		The privileges will be shown in tree format.

		This command supports listing privileges across multiple clusters, by using --cluster.
		If not set, the control plane will be used. This feature requires cluster-gateway to be
		properly setup to use. 

		The privileges are collected through listing all ClusterRoleBinding and RoleBinding,
		following the Kubernetes RBAC Authorization. Other authorization mechanism is not supported
		now. See https://kubernetes.io/docs/reference/access-authn-authz/rbac/ for details.
		
		The ClusterRoleBinding and RoleBinding that matches the specified identity will be 
		tracked. Related ClusterRoles and Roles are retrieved and the contained PolicyRules are
		demonstrated.`))

	listPrivilegesExample = templates.Examples(i18n.T(`
		# List privileges for User alice in the control plane
		vela auth list-privileges --user alice
		
		# List privileges for Group org:dev-team in the control plane
		vela auth list-privileges --group org:dev-team

		# List privileges for User bob with Groups org:dev-team and org:test-team in the control plane and managed cluster example-cluster
		vela auth list-privileges --user bob --group org:dev-team --group org:test-team --cluster local --cluster example-cluster
		
		# List privileges for ServiceAccount example-sa in demo namespace in multiple managed clusters 
		vela auth list-privileges --serviceaccount example-sa -n demo --cluster cluster-1 --cluster cluster-2

		# List privileges for identity in kubeconfig
		vela auth list-privileges --kubeconfig ./example.kubeconfig --cluster local --cluster cluster-1`))
)

// NewListPrivilegesCommand list privileges for given identity
func NewListPrivilegesCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	o := &ListPrivilegesOptions{IOStreams: streams}
	cmd := &cobra.Command{
		Use:                   "list-privileges",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("List privileges for user/group/serviceaccount"),
		Long:                  listPrivilegesLong,
		Example:               listPrivilegesExample,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			o.Complete(f, cmd)
			cmdutil.CheckErr(o.Validate(f, cmd))
			cmdutil.CheckErr(o.Run(f, cmd))
		},
	}
	cmd.Flags().StringVarP(&o.User, "user", "u", o.User, "The user to list privileges.")
	cmd.Flags().StringSliceVarP(&o.Groups, "group", "g", o.Groups, "The group to list privileges. Can be set together with --user.")
	cmd.Flags().StringVarP(&o.ServiceAccount, "serviceaccount", "", o.ServiceAccount, "The serviceaccount to list privileges. Cannot be set with --user and --group.")
	cmd.Flags().StringVarP(&o.KubeConfig, "kubeconfig", "", o.KubeConfig, "The kubeconfig to list privileges. If set, it will override all the other identity flags.")
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"serviceaccount", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if strings.TrimSpace(o.User) != "" {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			namespace := velacmd.GetNamespace(f, cmd)
			return velacmd.GetServiceAccountForCompletion(cmd.Context(), f, namespace, toComplete)
		}))

	return velacmd.NewCommandBuilder(f, cmd).
		WithNamespaceFlag(velacmd.UsageOption("The namespace of the serviceaccount. This flag only works when `--serviceaccount` is set.")).
		WithClusterFlag(velacmd.UsageOption("The cluster to list privileges. If not set, the command will list privileges in the control plane.")).
		WithStreams(streams).
		WithResponsiveWriter().
		Build()
}

// GrantPrivilegesOptions options for grant privileges
type GrantPrivilegesOptions struct {
	auth.Identity
	KubeConfig      string
	GrantNamespaces []string
	GrantClusters   []string
	ReadOnly        bool
	CreateNamespace bool

	util.IOStreams
}

// Complete .
func (opt *GrantPrivilegesOptions) Complete(f velacmd.Factory, cmd *cobra.Command) {
	if opt.KubeConfig != "" {
		identity, err := auth.ReadIdentityFromKubeConfig(opt.KubeConfig)
		cmdutil.CheckErr(err)
		opt.Identity = *identity
		opt.Identity.Groups = nil
	}
	if opt.Identity.ServiceAccount != "" {
		opt.Identity.ServiceAccountNamespace = velacmd.GetNamespace(f, cmd)
	}
	opt.Regularize()
	if len(opt.GrantClusters) == 0 {
		opt.GrantClusters = []string{types.ClusterLocalName}
	}
}

// Validate .
func (opt *GrantPrivilegesOptions) Validate(f velacmd.Factory, cmd *cobra.Command) error {
	if opt.User == "" && len(opt.Groups) == 0 && opt.ServiceAccount == "" {
		return fmt.Errorf("at least one idenity (user/group/serviceaccount) should be set")
	}
	for _, cluster := range opt.GrantClusters {
		if _, err := multicluster.NewClusterClient(f.Client()).Get(cmd.Context(), cluster); err != nil {
			return fmt.Errorf("failed to find cluster %s: %w", cluster, err)
		}
		if !opt.CreateNamespace {
			for _, namespace := range opt.GrantNamespaces {
				if err := f.Client().Get(multicluster.ContextWithClusterName(cmd.Context(), cluster), apitypes.NamespacedName{Name: namespace}, &corev1.Namespace{}); err != nil {
					return fmt.Errorf("failed to find namespace %s in cluster %s: %w", namespace, cluster, err)
				}
			}
		}
	}
	return nil
}

// Run .
func (opt *GrantPrivilegesOptions) Run(f velacmd.Factory, cmd *cobra.Command) error {
	ctx := cmd.Context()
	if opt.CreateNamespace {
		for _, cluster := range opt.GrantClusters {
			if _, err := multicluster.NewClusterClient(f.Client()).Get(cmd.Context(), cluster); err != nil {
				return fmt.Errorf("failed to find cluster %s: %w", cluster, err)
			}
			for _, namespace := range opt.GrantNamespaces {
				_ctx := multicluster.ContextWithClusterName(cmd.Context(), cluster)
				ns := &corev1.Namespace{}
				if err := f.Client().Get(_ctx, apitypes.NamespacedName{Name: namespace}, ns); err != nil {
					if kerrors.IsNotFound(err) {
						ns.SetName(namespace)
						if err = f.Client().Create(_ctx, ns); err != nil {
							return fmt.Errorf("failed to create namespace %s in cluster %s: %w", namespace, cluster, err)
						}
						continue
					}
					return fmt.Errorf("failed to find namespace %s in cluster %s: %w", namespace, cluster, err)
				}
			}
		}
	}
	var privileges []auth.PrivilegeDescription
	for _, cluster := range opt.GrantClusters {
		for _, namespace := range opt.GrantNamespaces {
			privileges = append(privileges, &auth.ScopedPrivilege{Cluster: cluster, Namespace: namespace, ReadOnly: opt.ReadOnly})
		}
		if len(opt.GrantNamespaces) == 0 {
			privileges = append(privileges, &auth.ScopedPrivilege{Cluster: cluster, ReadOnly: opt.ReadOnly})
		}
	}
	if err := auth.GrantPrivileges(ctx, f.Client(), privileges, &opt.Identity, opt.IOStreams.Out); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(opt.IOStreams.Out, "Privileges granted.\n")
	return nil
}

var (
	grantPrivilegesLong = templates.LongDesc(i18n.T(`
		Grant privileges for user

		Grant privileges to user/group/serviceaccount. By using --for-namespace and --for-cluster,
		you can grant all read/write privileges for all resources in the specified namespace and 
		cluster. If --for-namespace is not set, the privileges will be granted cluster-wide. 

		Setting --create-namespace will automatically create namespace if the namespace of the
		granted privilege does not exists. By default, this flag is not enabled and errors will be
		returned if the namespace is not found in the corresponding cluster.

		Setting --readonly will only grant read privileges for all resources in the destination. This
		can be useful if you want to give somebody the privileges to view resources but do not want to
		allow them to edit any resource.
		
		If multiple identity information are set, all the identity information will be bond to the
		intended privileges respectively.

		If --kubeconfig is set, the user/serviceaccount information in the kubeconfig will be used as
		the identity to grant privileges. Groups will be ignored.`))

	grantPrivilegesExample = templates.Examples(i18n.T(`
		# Grant privileges for User alice in the namespace demo of the control plane
		vela auth grant-privileges --user alice --for-namespace demo

		# Grant privileges for User alice in the namespace demo in cluster-1, create demo namespace if not exist
		vela auth grant-privileges --user alice --for-namespace demo --for-cluster cluster-1 --create-namespace
		
		# Grant cluster-scoped privileges for Group org:dev-team in the control plane
		vela auth grant-privileges --group org:dev-team

		# Grant privileges for Group org:dev-team and org:test-team in the namespace test on the control plane and managed cluster example-cluster
		vela auth grant-privileges --group org:dev-team --group org:test-team --for-namespace test --for-cluster local --for-cluster example-cluster
		
		# Grant read privileges for ServiceAccount observer in test namespace on the control plane
		vela auth grant-privileges --serviceaccount observer -n test --for-namespace test --readonly

		# Grant privileges for identity in kubeconfig in cluster-1
		vela auth grant-privileges --kubeconfig ./example.kubeconfig --for-cluster cluster-1`))
)

// NewGrantPrivilegesCommand grant privileges to given identity
func NewGrantPrivilegesCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	o := &GrantPrivilegesOptions{IOStreams: streams}
	cmd := &cobra.Command{
		Use:                   "grant-privileges",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Grant privileges for user/group/serviceaccount"),
		Long:                  grantPrivilegesLong,
		Example:               grantPrivilegesExample,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			o.Complete(f, cmd)
			cmdutil.CheckErr(o.Validate(f, cmd))
			cmdutil.CheckErr(o.Run(f, cmd))
		},
	}
	cmd.Flags().StringVarP(&o.User, "user", "u", o.User, "The user to grant privileges.")
	cmd.Flags().StringSliceVarP(&o.Groups, "group", "g", o.Groups, "The group to grant privileges.")
	cmd.Flags().StringVarP(&o.ServiceAccount, "serviceaccount", "", o.ServiceAccount, "The serviceaccount to grant privileges.")
	cmd.Flags().StringVarP(&o.KubeConfig, "kubeconfig", "", o.KubeConfig, "The kubeconfig to grant privileges. If set, it will override all the other identity flags.")
	cmd.Flags().StringSliceVarP(&o.GrantClusters, "for-cluster", "", o.GrantClusters, "The clusters privileges to grant. If empty, the control plane will be used.")
	cmd.Flags().StringSliceVarP(&o.GrantNamespaces, "for-namespace", "", o.GrantNamespaces, "The namespaces privileges to grant. If empty, cluster-scoped privileges will be granted.")
	cmd.Flags().BoolVarP(&o.ReadOnly, "readonly", "", o.ReadOnly, "If set, only read privileges of resources will be granted. Otherwise, read/write privileges will be granted.")
	cmd.Flags().BoolVarP(&o.CreateNamespace, "create-namespace", "", o.CreateNamespace, "If set, non-exist namespace will be created automatically.")
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"serviceaccount", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if strings.TrimSpace(o.User) != "" {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			namespace := velacmd.GetNamespace(f, cmd)
			return velacmd.GetServiceAccountForCompletion(cmd.Context(), f, namespace, toComplete)
		}))

	return velacmd.NewCommandBuilder(f, cmd).
		WithNamespaceFlag(velacmd.UsageOption("The namespace of the serviceaccount. This flag only works when `--serviceaccount` is set.")).
		WithStreams(streams).
		WithResponsiveWriter().
		Build()
}
