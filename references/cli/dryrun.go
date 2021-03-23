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

	corev1alpha2 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	appfile2 "github.com/oam-dev/kubevela/references/appfile"
)

type dryRunOptions struct {
	cmdutil.IOStreams
	applicationFile string
	definitionFile  string
}

// NewDryRunCommand creates `dry-run` command
func NewDryRunCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	o := &dryRunOptions{IOStreams: ioStreams}
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
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}

			velaEnv, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			if o.definitionFile != "" {
				objs, err := ReadObjectsFromFile(o.definitionFile)
				if err != nil {
					return err
				}
				for _, obj := range objs {
					if obj.GetNamespace() == "" {
						obj.SetNamespace(velaEnv.Namespace)
					}
				}
				if err = appfile2.CreateOrUpdateObjects(context.TODO(), newClient, objs); err != nil {
					return err
				}
			}
			pd, err := c.GetPackageDiscover()
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

			parser := appfile.NewApplicationParser(newClient, dm, pd)

			ctx := oamutil.SetNamespaceInCtx(context.Background(), velaEnv.Namespace)

			appFile, err := parser.GenerateAppFile(ctx, app.Name, app)
			if err != nil {
				return errors.WithMessage(err, "generate appFile")
			}

			ac, comps, err := parser.GenerateApplicationConfiguration(appFile, app.Namespace)
			if err != nil {
				return errors.WithMessage(err, "generate OAM objects")
			}
			var buff = bytes.Buffer{}
			var components = make(map[string]runtime.RawExtension)
			for _, comp := range comps {
				components[comp.Name] = comp.Spec.Workload
			}
			for _, c := range ac.Spec.Components {
				buff.Write([]byte(fmt.Sprintf("---\n# Application(%s) -- Comopnent(%s) \n---\n\n", ac.Name, c.ComponentName)))
				result, err := yaml.Marshal(components[c.ComponentName])
				if err != nil {
					return errors.WithMessage(err, "marshal result for component "+c.ComponentName+" object in yaml format")
				}
				buff.Write(result)
				buff.Write([]byte("\n---\n"))
				for _, t := range c.Traits {
					result, err := yaml.Marshal(t.Trait)
					if err != nil {
						return errors.WithMessage(err, "marshal result for component "+c.ComponentName+" object in yaml format")
					}
					buff.Write(result)
					buff.Write([]byte("\n---\n"))
				}
				buff.Write([]byte("\n"))
			}
			o.Info(buff.String())
			return nil
		},
	}

	cmd.Flags().StringVarP(&o.applicationFile, "file", "f", "./app.yaml", "application file name")
	cmd.Flags().StringVarP(&o.definitionFile, "definition", "d", "", "specify a definition file or directory, it will automatically applied to the K8s cluster")
	cmd.SetOut(ioStreams.Out)
	return cmd
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
