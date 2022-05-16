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
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/auth"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	cmdutil "github.com/oam-dev/kubevela/pkg/cmd/util"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

// CreateCommandGroup commands for create resources or configuration
func CreateCommandGroup(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: i18n.T("Create resource or configuration"),
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
	}
	cmd.AddCommand(NewCreateKubeConfigCommand(f, streams))
	return cmd
}

// CreateKubeConfigOptions options for create kubeconfig
type CreateKubeConfigOptions struct {
	User                    string
	Groups                  []string
	ServiceAccountName      string
	ServiceAccountNamespace string

	util.IOStreams
}

func (opt *CreateKubeConfigOptions) options() []auth.KubeConfigCreateOption {
	var opts []auth.KubeConfigCreateOption
	if opt.User != "" {
		opts = append(opts, auth.KubeConfigWithUserCreateOption(opt.User))
	}
	for _, group := range opt.Groups {
		opts = append(opts, auth.KubeConfigWithGroupCreateOption(group))
	}
	if opt.ServiceAccountName != "" {
		opts = append(opts, auth.KubeConfigWithServiceAccount{
			Name:      opt.ServiceAccountName,
			Namespace: opt.ServiceAccountNamespace,
		})
	}
	return opts
}

// Complete .
func (opt *CreateKubeConfigOptions) Complete(f velacmd.Factory, cmd *cobra.Command) {
	opt.User = strings.TrimSpace(opt.User)
	groupMap := map[string]struct{}{}
	var groups []string
	for _, group := range opt.Groups {
		group = strings.TrimSpace(group)
		if _, found := groupMap[group]; !found {
			groupMap[group] = struct{}{}
			groups = append(groups, group)
		}
	}
	opt.Groups = groups
	opt.ServiceAccountName = strings.TrimSpace(opt.ServiceAccountName)
	if opt.ServiceAccountName != "" {
		if ns := velacmd.GetNamespace(f, cmd); ns != "" {
			opt.ServiceAccountNamespace = ns
		} else {
			opt.ServiceAccountNamespace = corev1.NamespaceDefault
		}
	}
}

// Validate .
func (opt *CreateKubeConfigOptions) Validate() error {
	if opt.User == "" && opt.ServiceAccountName == "" {
		return errors.Errorf("either `user` or `serviceaccount` should be set")
	}
	if opt.User != "" && opt.ServiceAccountName != "" {
		return errors.Errorf("cannot set `user` and `serviceaccount` at the same time")
	}
	if opt.User == "" && len(opt.Groups) > 0 {
		return errors.Errorf("cannot set groups when user is not set")
	}
	if opt.ServiceAccountName == "" && opt.ServiceAccountNamespace != "" {
		return errors.Errorf("cannot set serviceaccount namespace when serviceaccount is not set")
	}
	return nil
}

// Run .
func (opt *CreateKubeConfigOptions) Run(f velacmd.Factory) error {
	ctx := context.Background()
	cli, err := kubernetes.NewForConfig(f.Config())
	if err != nil {
		return err
	}
	cfg, err := clientcmd.NewDefaultPathOptions().GetStartingConfig()
	if err != nil {
		return err
	}
	cfg, err = auth.CreateKubeConfig(ctx, cli, cfg, opt.IOStreams, opt.options()...)
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
	createKubeConfigLong = templates.LongDesc(i18n.T(`
		Create kubeconfig for user

		Create a new kubeconfig with specified identity. By default, the generated kubeconfig 
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

	createKubeConfigExample = templates.Examples(i18n.T(`
		# Create a kubeconfig with provided user
		vela create kubeconfig --user new-user
		
		# Create a kubeconfig with provided user and group
		vela create kubeconfig --user new-user --group kubevela:developer
		
		# Create a kubeconfig with provided user and groups
		vela create kubeconfig --user new-user --group kubevela:developer --group my-org:my-team

		# Create a kubeconfig with provided serviceaccount
		vela create kubeconfig --serviceaccount default -n demo`))
)

// NewCreateKubeConfigCommand create kubeconfig for given user and groups
func NewCreateKubeConfigCommand(f velacmd.Factory, streams util.IOStreams) *cobra.Command {
	o := &CreateKubeConfigOptions{IOStreams: streams}
	cmd := &cobra.Command{
		Use:                   "kubeconfig",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Create kubeconfig for user"),
		Long:                  createKubeConfigLong,
		Example:               createKubeConfigExample,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
		Args: cobra.ExactValidArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			o.Complete(f, cmd)
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run(f))
		},
	}
	cmd.Flags().StringVarP(&o.User, "user", "u", o.User, "The user of the generated kubeconfig. If set, an X509-based kubeconfig will be intended to create. It will be embedded as the Subject in the X509 certificate.")
	cmd.Flags().StringSliceVarP(&o.Groups, "group", "g", o.Groups, "The groups of the generated kubeconfig. This flag only works when `--user` is set. It will be embedded as the Organization in the X509 certificate.")
	cmd.Flags().StringVarP(&o.ServiceAccountName, "serviceaccount", "", o.ServiceAccountName, "The serviceaccount of the generated kubeconfig. If set, a kubeconfig will be generated based on the secret token of the serviceaccount. Cannot be set when `--user` presents.")
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"serviceaccount", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if strings.TrimSpace(o.User) != "" {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			namespace := velacmd.GetNamespace(f, cmd)
			return velacmd.GetServiceAccountForCompletion(cmd.Context(), f, namespace, toComplete)
		}))

	return velacmd.NewCommandBuilder(f, cmd).
		WithNamespaceFlag(velacmd.NamespaceFlagUsageOption("The namespace of the serviceaccount. This flag only works when `--serviceaccount` is set.")).
		WithStreams(streams).
		WithResponsiveWriter().
		Build()
}
