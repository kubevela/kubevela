package cli

import (
	"fmt"
	"github.com/pkg/errors"

	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"

	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/cli/top/view"
)

func NewTopCommand(c common.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "top",
		Short: "Launch UI to display system performance.",
		Long:  "Launch UI to display system information and resource status of application.",
		Example: `  # Launch UI to display system information and resource status of application
  vela top`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return launchUI(c, cmd)
		},
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
	}
	return cmd
}

func launchUI(c common.Args, _ *cobra.Command) error {
	k8sClient, err := c.GetClient()
	if err != nil {
		return fmt.Errorf("cannot get k8s client: %w", err)
	}
	restConfig, err := c.GetConfig()
	if err != nil {
		return errors.Wrapf(err, "failed to get kube config, You can set KUBECONFIG env or make file ~/.kube/config")
	}
	app := view.NewApp(k8sClient, restConfig)
	app.Init()

	return app.Run()
}
