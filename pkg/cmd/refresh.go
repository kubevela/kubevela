package cmd

import (
	"context"
	"path/filepath"

	"github.com/cloud-native-application/rudrx/api/types"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/cloud-native-application/rudrx/pkg/plugins"
	"github.com/cloud-native-application/rudrx/pkg/utils/system"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewRefreshCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "refresh",
		DisableFlagsInUseLine: true,
		Short:                 "Sync definition from cluster",
		Long:                  "Refresh and sync definition files from cluster",
		Example:               `vela refresh`,
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
	dir, _ := system.GetDefinitionDir()

	ioStreams.Info("syncing workload definitions from cluster...")
	templates, err := plugins.GetWorkloadsFromCluster(ctx, types.DefaultOAMNS, c, dir, nil)
	if err != nil {
		return err
	}
	workloadDir := filepath.Join(dir, "workloads")
	system.StatAndCreate(workloadDir)
	ioStreams.Infof("get %d workload definitions from cluster, syncing to %s...", len(templates), workloadDir)
	successNum := plugins.SinkTemp2Local(templates, workloadDir)
	ioStreams.Infof("%d workload definitions successfully synced\n", successNum)

	ioStreams.Info("syncing trait definitions from cluster...")
	templates, err = plugins.GetTraitsFromCluster(ctx, types.DefaultOAMNS, c, dir, nil)
	if err != nil {
		return err
	}
	traitDir := filepath.Join(dir, "traits")
	system.StatAndCreate(traitDir)
	ioStreams.Infof("get %d trait definitions from cluster, syncing to %s...", len(templates), traitDir)
	successNum = plugins.SinkTemp2Local(templates, traitDir)
	ioStreams.Infof("%d trait definitions successfully synced\n", successNum)
	return nil
}
