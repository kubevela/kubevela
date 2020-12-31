package commands

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/application"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/server/apis"
	"github.com/oam-dev/kubevela/pkg/serverlib"
)

// NewListCommand creates `ls` command and its nested children command
func NewListCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "ls",
		Aliases:               []string{"list"},
		DisableFlagsInUseLine: true,
		Short:                 "List services",
		Long:                  "List services of all applications",
		Example:               `vela ls`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
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
	cmd.PersistentFlags().StringP(App, "", "", "specify the name of application")
	return cmd
}

func printComponentList(ctx context.Context, c client.Client, appName string, env *types.EnvMeta, ioStreams cmdutil.IOStreams) {
	deployedComponentList, err := serverlib.ListComponents(ctx, c, serverlib.Option{
		AppName:   appName,
		Namespace: env.Namespace,
	})
	if err != nil {
		ioStreams.Infof("listing services: %s\n", err)
		return
	}

	fetcher := func(name string) (*v1alpha2.Application, error) {
		var app = new(v1alpha2.Application)
		err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: env.Namespace}, app)
		return app, err
	}
	all := mergeStagingComponents(deployedComponentList, env, ioStreams, fetcher)
	table := newUITable()
	table.AddRow("SERVICE", "APP", "TYPE", "TRAITS", "STATUS", "CREATED-TIME")
	for _, a := range all {
		traitAlias := strings.Join(a.TraitNames, ",")
		table.AddRow(a.Name, a.App, a.WorkloadName, traitAlias, a.Status, a.CreatedTime)
	}
	ioStreams.Info(table.String())
}

func mergeStagingComponents(deployed []apis.ComponentMeta, env *types.EnvMeta, ioStreams cmdutil.IOStreams, fetcher func(name string) (*v1alpha2.Application, error)) []apis.ComponentMeta {
	localApps, err := application.List(env.Name)
	if err != nil {
		ioStreams.Error("list application err", err)
		return deployed
	}
	var all []apis.ComponentMeta
	for _, app := range localApps {
		appl, err := fetcher(app.Name)
		if err != nil {
			ioStreams.Errorf("fetch app %s err %v\n", app.Name, err)
			continue
		}
		for _, c := range appl.Spec.Components {
			traits := []string{}
			for _, t := range c.Traits {
				traits = append(traits, t.Name)
			}
			compMeta, exist := GetCompMeta(deployed, app.Name, c.Name)
			if !exist {
				all = append(all, apis.ComponentMeta{
					Name:         c.Name,
					App:          app.Name,
					WorkloadName: c.WorkloadType,
					TraitNames:   traits,
					Status:       types.StatusStaging,
					CreatedTime:  app.CreateTime.String(),
				})
				continue
			}
			compMeta.TraitNames = traits
			compMeta.WorkloadName = c.WorkloadType
			if appl.Status.Phase != v1alpha2.ApplicationRunning {
				compMeta.Status = types.StatusStaging
			}
			all = append(all, compMeta)
		}
	}
	return all
}

// GetCompMeta gets meta of a component
func GetCompMeta(deployed []apis.ComponentMeta, appName, compName string) (apis.ComponentMeta, bool) {
	for _, v := range deployed {
		if v.Name == compName && v.App == appName {
			return v, true
		}
	}
	return apis.ComponentMeta{}, false
}
