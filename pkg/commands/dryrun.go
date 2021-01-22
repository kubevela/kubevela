package commands

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	corev1alpha2 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	appCtr "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type dryRunOptions struct {
	cmdutil.IOStreams
	applicationFile string
}

// NewDryRunCommand creates `dry-run` command
func NewDryRunCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	o := &dryRunOptions{IOStreams: ioStreams}
	cmd := &cobra.Command{
		Use:                   "dry-run",
		DisableFlagsInUseLine: true,
		Short:                 "Dry Run an application, and output the conversion result to stdout",
		Long:                  "Dry Run an application, and output the conversion result to stdout",
		Example:               "vela dry-run",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}

			dm, err := discoverymapper.New(c.Config)
			if err != nil {
				return err
			}

			app, err := readApplicationFromFile(o.applicationFile)
			if err != nil {
				return errors.WithMessagef(err, "read application file: %s", o.applicationFile)
			}

			parser := appCtr.NewApplicationParser(newClient, dm)

			appFile, err := parser.GenerateAppFile(app.Name, app)
			if err != nil {
				return errors.WithMessage(err, "generate appFile")
			}

			ac, comps, err := parser.GenerateApplicationConfiguration(appFile, app.Namespace)
			if err != nil {
				return errors.WithMessage(err, "generate OAM objects")
			}

			var outs = []interface{}{ac}
			for index := range comps {
				outs = append(outs, comps[index])
			}

			result, err := yaml.Marshal(outs)
			if err != nil {
				return errors.WithMessage(err, "marshal result object in yaml format")
			}
			o.Info(string(result))
			return nil
		},
	}

	cmd.Flags().StringVarP(&o.applicationFile, "file", "f", "./app.yaml", "application file name")
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func readApplicationFromFile(filename string) (*corev1alpha2.Application, error) {

	fileContent, err := ioutil.ReadFile(filepath.Clean(filename))
	if err != nil {
		return nil, err
	}

	fileType := filepath.Ext(filename)
	switch fileType {
	case ".yaml", ".yml":
		fileContent, err = yaml.YAMLToJSON(fileContent)
		if err != nil {
			return nil, err
		}
	}

	app := new(corev1alpha2.Application)
	err = json.Unmarshal(fileContent, app)
	return app, err
}
