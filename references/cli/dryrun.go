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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	corev1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/appfile/dryrun"
)

// DryRunCmdOptions contains dry-run cmd options
type DryRunCmdOptions struct {
	cmdutil.IOStreams
	ApplicationFile string
	DefinitionFile  string
}

// NewDryRunCommand creates `dry-run` command
func NewDryRunCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var namespace string
	o := &DryRunCmdOptions{IOStreams: ioStreams}
	cmd := &cobra.Command{
		Use:                   "dry-run",
		DisableFlagsInUseLine: true,
		Short:                 "Dry Run an application, and output the K8s resources as result to stdout",
		Long:                  "Dry Run an application, and output the K8s resources as result to stdout, only CUE template supported for now",
		Example:               "vela dry-run",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
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
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "specify namespace of the definition file, by default is default namespace")
	cmd.SetOut(ioStreams.Out)
	return cmd
}

// DryRunApplication will dry-run an application and return the render result
func DryRunApplication(cmdOption *DryRunCmdOptions, c common.Args, namespace string) (bytes.Buffer, error) {
	var buff = bytes.Buffer{}

	newClient, err := c.GetClient()
	if err != nil {
		return buff, err
	}
	objs := []oam.Object{}
	if cmdOption.DefinitionFile != "" {
		objs, err = ReadObjectsFromFile(cmdOption.DefinitionFile)
		if err != nil {
			return buff, err
		}
	}
	pd, err := c.GetPackageDiscover()
	if err != nil {
		return buff, err
	}

	dm, err := discoverymapper.New(c.Config)
	if err != nil {
		return buff, err
	}

	app, err := readApplicationFromFile(cmdOption.ApplicationFile)
	if err != nil {
		return buff, errors.WithMessagef(err, "read application file: %s", cmdOption.ApplicationFile)
	}

	dryRunOpt := dryrun.NewDryRunOption(newClient, dm, pd, objs)
	ctx := oamutil.SetNamespaceInCtx(context.Background(), namespace)
	ac, comps, err := dryRunOpt.ExecuteDryRun(ctx, app)
	if err != nil {
		return buff, errors.WithMessage(err, "generate OAM objects")
	}

	var components = make(map[string]runtime.RawExtension)
	for _, comp := range comps {
		components[comp.Name] = comp.Spec.Workload
	}
	for _, c := range ac.Spec.Components {
		buff.Write([]byte(fmt.Sprintf("---\n# Application(%s) -- Comopnent(%s) \n---\n\n", ac.Name, c.ComponentName)))
		result, err := yaml.Marshal(components[c.ComponentName])
		if err != nil {
			return buff, errors.WithMessage(err, "marshal result for component "+c.ComponentName+" object in yaml format")
		}
		buff.Write(result)
		buff.Write([]byte("\n---\n"))
		for _, t := range c.Traits {
			result, err := yaml.Marshal(t.Trait)
			if err != nil {
				return buff, errors.WithMessage(err, "marshal result for component "+c.ComponentName+" object in yaml format")
			}
			buff.Write(result)
			buff.Write([]byte("\n---\n"))
		}
		buff.Write([]byte("\n"))
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
	fis, err := ioutil.ReadDir(path)
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

	app := new(corev1beta1.Application)
	err = json.Unmarshal(fileContent, app)
	return app, err
}
