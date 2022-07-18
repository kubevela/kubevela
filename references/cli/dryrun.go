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

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	corev1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile/dryrun"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// DryRunCmdOptions contains dry-run cmd options
type DryRunCmdOptions struct {
	cmdutil.IOStreams
	ApplicationFile string
	DefinitionFile  string
	OfflineMode     bool
}

// NewDryRunCommand creates `dry-run` command
func NewDryRunCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	o := &DryRunCmdOptions{IOStreams: ioStreams}
	cmd := &cobra.Command{
		Use:                   "dry-run",
		DisableFlagsInUseLine: true,
		Short:                 "Dry Run an application, and output the K8s resources as result to stdout",
		Long: `Dry-run application locally, render the Kubernetes resources as result to stdout.
	vela dry-run -d <definition file or directory> -f /path/to/app.yaml

You can also specify a remote url for app:
	vela dry-run -d <definition file or directory> -f https://<remote-host>/app.yaml
`,
		Example: "vela dry-run",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				// We need to return an error only if not in offline mode
				if !o.OfflineMode {
					return err
				}

				// Set the namespace to default to match behaviour of `GetFlagNamespaceOrEnv`
				namespace = "default"
			}

			buff, err := DryRunApplication(o, c, namespace)
			if err != nil {
				return err
			}
			o.Info(buff.String())
			return nil
		},
	}

	cmd.Flags().StringVarP(&o.ApplicationFile, "file", "f", "./app.yaml", "application file name")
	cmd.Flags().StringVarP(&o.DefinitionFile, "definition", "d", "", "specify a definition file or directory, it will only be used in dry-run rather than applied to K8s cluster")
	cmd.Flags().BoolVar(&o.OfflineMode, "offline", false, "Run `dry-run` in offline / local mode, all validation steps will be skipped")
	addNamespaceAndEnvArg(cmd)
	cmd.SetOut(ioStreams.Out)
	return cmd
}

// DryRunApplication will dry-run an application and return the render result
func DryRunApplication(cmdOption *DryRunCmdOptions, c common.Args, namespace string) (bytes.Buffer, error) {
	var err error
	var buff = bytes.Buffer{}

	objs := []oam.Object{}
	if cmdOption.DefinitionFile != "" {
		objs, err = ReadObjectsFromFile(cmdOption.DefinitionFile)
		if err != nil {
			return buff, err
		}
	}

	// Load a kubernetes client
	var newClient client.Client
	if cmdOption.OfflineMode {
		// We will load a fake client with all the objects present in the definitions file preloaded
		newClient, err = c.GetFakeClient(objs)
	} else {
		// Load an actual client here
		newClient, err = c.GetClient()
	}
	if err != nil {
		return buff, err
	}

	pd, err := c.GetPackageDiscover()
	if err != nil {
		return buff, err
	}
	config, err := c.GetConfig()
	if err != nil {
		return buff, err
	}
	dm, err := discoverymapper.New(config)
	if err != nil {
		return buff, err
	}

	dryRunOpt := dryrun.NewDryRunOption(newClient, config, dm, pd, objs, false)
	ctx := oamutil.SetNamespaceInCtx(context.Background(), namespace)

	// Perform validation only if not in offline mode
	if !cmdOption.OfflineMode {
		err = dryRunOpt.ValidateApp(ctx, cmdOption.ApplicationFile)
		if err != nil {
			return buff, errors.WithMessagef(err, "validate application: %s by dry-run", cmdOption.ApplicationFile)
		}
	}

	app, err := readApplicationFromFile(cmdOption.ApplicationFile)
	if err != nil {
		return buff, errors.WithMessagef(err, "read application file: %s", cmdOption.ApplicationFile)
	}

	comps, policies, err := dryRunOpt.ExecuteDryRun(ctx, app)
	if err != nil {
		return buff, errors.WithMessage(err, "generate OAM objects")
	}

	if err = dryRunOpt.PrintDryRun(&buff, app.Name, comps, policies); err != nil {
		return buff, err
	}
	return buff, nil
}

// ReadObjectsFromFile will read objects from file or dir in the format of yaml
func ReadObjectsFromFile(path string) ([]oam.Object, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		obj := &unstructured.Unstructured{}
		err = common.ReadYamlToObject(path, obj)
		if err != nil {
			return nil, err
		}
		return []oam.Object{obj}, nil
	}

	var objs []oam.Object
	//nolint:gosec
	fis, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	for _, fi := range fis {
		if fi.IsDir() {
			continue
		}
		fileType := filepath.Ext(fi.Name())
		if fileType != ".yaml" && fileType != ".yml" {
			continue
		}
		obj := &unstructured.Unstructured{}
		err = common.ReadYamlToObject(filepath.Join(path, fi.Name()), obj)
		if err != nil {
			return nil, err
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

func readApplicationFromFile(filename string) (*corev1beta1.Application, error) {
	fileContent, err := utils.ReadRemoteOrLocalPath(filename, true)
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

	app := new(corev1beta1.Application)
	err = json.Unmarshal(fileContent, app)
	return app, err
}
