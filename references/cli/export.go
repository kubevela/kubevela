package cli

import (
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

// NewExportCommand will create command for exporting deploy manifests from an AppFile
func NewExportCommand(c types.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "export",
		DisableFlagsInUseLine: true,
		Short:                 "Export deploy manifests from appfile",
		Long:                  "Export deploy manifests from appfile",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			o := &common.AppfileOptions{
				IO:  ioStream,
				Env: &types.EnvMeta{},
			}
			filePath, err := cmd.Flags().GetString(appFilePath)
			if err != nil {
				return err
			}
			_, data, err := o.Export(filePath, true)
			if err != nil {
				return err
			}
			_, err = ioStream.Out.Write(data)
			return err
		},
	}
	cmd.SetOut(ioStream.Out)

	cmd.Flags().StringP(appFilePath, "f", "", "specify file path for appfile")
	return cmd
}
