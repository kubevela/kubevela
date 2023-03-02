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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/schema"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

// NewUISchemaCommand creates `uischema` command
func NewUISchemaCommand(c common.Args, order string, ioStreams util.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "uischema",
		Aliases: []string{"ui"},
		Short:   "Manage UI schema for addons.",
		Long:    "Manage UI schema for addons.",
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeExtension,
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "apply <ui schema file/dir path>",
		Args:  cobra.ExactArgs(1),
		Long:  "apply UI schema from a file or dir",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeExtension,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("please provider the ui schema file or dir path")
			}
			allUISchemaFiles, err := loadUISchemaFiles(args[0])
			if err != nil {
				return err
			}
			client, err := c.GetClient()
			if err != nil {
				return err
			}
			for _, file := range allUISchemaFiles {
				if err := applyUISchemaFile(client, file); err != nil {
					ioStreams.Errorf("apply %s failure %s \n", file, err.Error())
					continue
				}
				ioStreams.Infof("apply %s success \n", file)
			}
			return nil
		},
	})

	return cmd
}

func loadUISchemaFiles(setpath string) ([]string, error) {
	schema, err := os.Stat(setpath)
	if err != nil {
		return nil, err
	}
	var readDir func(string) ([]string, error)
	readDir = func(filepath string) ([]string, error) {
		files, err := os.ReadDir(filepath)
		if err != nil {
			return nil, err
		}
		var subFiles []string
		for _, f := range files {
			if f.IsDir() {
				files, err := readDir(path.Join(filepath, f.Name()))
				if err != nil {
					return nil, err
				}
				subFiles = append(subFiles, files...)
			} else if strings.HasSuffix(f.Name(), ".yaml") {
				subFiles = append(subFiles, path.Join(filepath, f.Name()))
			}
		}
		return subFiles, nil
	}
	var allUISchemaFiles = []string{}
	if schema.IsDir() {
		allUISchemaFiles, err = readDir(setpath)
		if err != nil {
			return nil, err
		}
	} else {
		allUISchemaFiles = append(allUISchemaFiles, setpath)
	}
	return allUISchemaFiles, nil
}

func applyUISchemaFile(client client.Client, uischemaFile string) error {
	cdata, err := os.ReadFile(filepath.Clean(uischemaFile))
	if err != nil {
		return err
	}
	fileBaseName := path.Base(uischemaFile)
	fileBaseNameWithoutExt := fileBaseName[:strings.Index(fileBaseName, ".")] // nolint

	infos := strings.SplitN(fileBaseNameWithoutExt, "-", func() int {
		if strings.Contains(fileBaseNameWithoutExt, "uischema") {
			return 3
		}
		return 2
	}())
	if len(infos) == 2 || len(infos) == 3 && infos[1] == "uischema" {
		name := infos[1]
		if len(infos) == 3 {
			name = infos[2]
		}
		err = addDefinitionUISchema(context.Background(), client, name, infos[0], string(cdata))
		if err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("file name is invalid")
}

func addDefinitionUISchema(ctx context.Context, client client.Client, name, defType, configRaw string) error {
	var uiParameters []*schema.UIParameter
	err := yaml.Unmarshal([]byte(configRaw), &uiParameters)
	if err != nil {
		return err
	}
	dataBate, err := json.Marshal(uiParameters)
	if err != nil {
		return err
	}

	var cm v1.ConfigMap
	if err := client.Get(ctx, k8stypes.NamespacedName{
		Namespace: types.DefaultKubeVelaNS,
		Name:      fmt.Sprintf("%s-uischema-%s", defType, name),
	}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			err = client.Create(ctx, &v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: types.DefaultKubeVelaNS,
					Name:      fmt.Sprintf("%s-uischema-%s", defType, name),
				},
				Data: map[string]string{
					types.UISchema: string(dataBate),
				},
			})
		}
		if err != nil {
			return err
		}
	} else {
		cm.Data[types.UISchema] = string(dataBate)
		err := client.Update(ctx, &cm)
		if err != nil {
			return err
		}
	}
	return nil
}
