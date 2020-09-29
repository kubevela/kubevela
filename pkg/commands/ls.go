package commands

import (
	"context"
	"strings"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	gocmp "github.com/google/go-cmp/cmp"
	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/application"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/server/apis"
)

func NewAppListCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "ls",
		DisableFlagsInUseLine: true,
		Short:                 "List applications",
		Long:                  "List applications with workloads, traits, status and created time",
		Example:               `vela app ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			return printApplicationList(ctx, newClient, env.Namespace, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.Flags().StringP(App, "a", "", "Application name")
	return cmd
}

func printApplicationList(ctx context.Context, c client.Client, namespace string, ioStreams cmdutil.IOStreams) error {
	var applicationList corev1alpha2.ApplicationConfigurationList

	err := c.List(ctx, &applicationList, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return err
	}
	for _, v := range applicationList.Items {
		ioStreams.Info(v.Name)
	}
	return nil
}

func NewCompListCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "ls",
		Aliases:               []string{"list"},
		DisableFlagsInUseLine: true,
		Short:                 "List applications",
		Long:                  "List applications with workloads, traits, status and created time",
		Example:               `vela comp ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			appName, err := cmd.Flags().GetString(App)
			if err != nil {
				return err
			}
			printComponentList(ctx, newClient, appName, env, ioStreams)
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	return cmd
}

func printComponentList(ctx context.Context, c client.Client, appName string, env *types.EnvMeta, ioStreams cmdutil.IOStreams) {
	deployedComponentList, err := oam.ListComponents(ctx, c, oam.Option{
		AppName:   appName,
		Namespace: env.Namespace,
	})
	if err != nil {
		ioStreams.Infof("listing Trait DefinitionPath hit an issue: %s\n", err)
		return
	}
	all := mergeStagingComponents(deployedComponentList, env, ioStreams)
	table := uitable.New()
	table.AddRow("NAME", "APP", "WORKLOAD", "TRAITS", "STATUS", "CREATED-TIME")
	for _, a := range all {
		traitAlias := strings.Join(a.TraitNames, ",")
		table.AddRow(a.Name, a.App, a.WorkloadName, traitAlias, a.Status, a.CreatedTime)
	}
	ioStreams.Info(table.String())
}

func mergeStagingComponents(deployed []apis.ComponentMeta, env *types.EnvMeta, ioStreams cmdutil.IOStreams) []apis.ComponentMeta {
	apps, err := application.List(env.Name)
	if err != nil {
		ioStreams.Error("list application err", err)
		return deployed
	}
	var all []apis.ComponentMeta
	for _, app := range apps {
		comps, appConfig, _, err := app.OAM(env)
		if err != nil {
			ioStreams.Errorf("convert app %s err %v\n", app.Name, err)
			continue
		}
		for _, c := range comps {
			traits, err := app.GetTraitNames(c.Name)
			if err != nil {
				ioStreams.Errorf("get traits from app %s %s err %v\n", app.Name, c.Name, err)
				continue
			}
			compMeta, exist := GetCompMeta(deployed, app.Name, c.Name)
			if !exist {
				all = append(all, apis.ComponentMeta{
					Name:         c.Name,
					App:          app.Name,
					WorkloadName: c.Annotations[types.AnnWorkloadDef],
					TraitNames:   traits,
					Status:       types.StatusStaging,
					CreatedTime:  app.CreateTime.String(),
				})
				continue
			}
			cspec := c.Spec.DeepCopy()
			cspec.Workload.Raw, _ = cspec.Workload.MarshalJSON()
			cspec.Workload.Object = nil
			aspec := appConfig.Spec.DeepCopy()
			for i, v := range aspec.Components {
				for j, t := range v.Traits {
					t.Trait.Raw, _ = t.Trait.MarshalJSON()
					t.Trait.Object = nil
					v.Traits[j] = t
				}
				aspec.Components[i] = v
			}
			if !gocmp.Equal(compMeta.Component.Spec, *cspec) || !gocmp.Equal(compMeta.AppConfig.Spec, *aspec) {
				compMeta.Status = types.StatusStaging
			}
			all = append(all, compMeta)
		}
	}
	return all
}

func GetCompMeta(deployed []apis.ComponentMeta, appName, compName string) (apis.ComponentMeta, bool) {
	for _, v := range deployed {
		if v.Name == compName && v.App == appName {
			return v, true
		}
	}
	return apis.ComponentMeta{}, false
}
