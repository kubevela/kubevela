package commands

import (
	"context"

	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/plugins"
	"github.com/oam-dev/kubevela/pkg/utils/system"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewRefreshCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "update",
		DisableFlagsInUseLine: true,
		Short:                 "Sync definition from cluster",
		Long:                  "Refresh and sync definition files from cluster",
		Example:               `vela system update`,
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			return RefreshDefinitions(ctx, newClient, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func RefreshDefinitions(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams) error {
	dir, _ := system.GetCapabilityDir()

	syncedTemplates := []types.Capability{}
	ioStreams.Info("syncing workload definitions from cluster...")
	templates, err := plugins.GetWorkloadsFromCluster(ctx, types.DefaultOAMNS, c, dir, nil)
	if err != nil {
		return err
	}
	syncedTemplates = append(syncedTemplates, templates...)
	ioStreams.Infof("get %d workload definition(s) from cluster, syncing...", len(templates))
	successNum := plugins.SinkTemp2Local(templates, dir)
	ioStreams.Infof("sync %d workload definition(s) successfully\n", successNum)

	ioStreams.Info("syncing trait definitions from cluster...")
	templates, err = plugins.GetTraitsFromCluster(ctx, types.DefaultOAMNS, c, dir, nil)
	if err != nil {
		return err
	}
	syncedTemplates = append(syncedTemplates, templates...)
	ioStreams.Infof("get %d trait definition(s) from cluster, syncing...", len(templates))
	successNum = plugins.SinkTemp2Local(templates, dir)
	ioStreams.Infof("sync %d trait definition(s) successfully\n", successNum)

	legacyNum := plugins.RemoveLegacyTemps(syncedTemplates, dir)
	ioStreams.Infof("remove %d legacy capability definition(s) successfully\n", legacyNum)
	return nil
}
