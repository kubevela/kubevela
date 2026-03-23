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
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

var (
	pkgName string
	pkgVersion string
	pkgFile string
	namespace string
)

// NewPackageObjectCommand creates 'package-object' command
func NewPackageObjectCommand(c common2.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	var isDiscover bool
	cmd := &cobra.Command{
		Use:     "package",
		Aliases: []string{"packages"},
		Short:   "List/get packages.",
		Long:    "List package types installed and discover more in registry.",
		Example: `vela package`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// parse label filter
			if label != "" {
				words := strings.Split(label, "=")
				if len(words) < 2 {
					return errors.New("label is invalid")
				}
				filter = createLabelFilter(words[0], words[1])
			}

			var registry Registry
			var err error
			if isDiscover {
				if regURL != "" {
					ioStreams.Infof("Listing package definition from url: %s\n", regURL)
					registry, err = NewRegistry(context.Background(), token, "temporary-registry", regURL)
					if err != nil {
						return errors.Wrap(err, "creating registry err, please check registry url")
					}
				} else {
					ioStreams.Infof("Listing component definition from registry: %s\n", regName)
					registry, err = GetRegistry(regName)
					if err != nil {
						return errors.Wrap(err, "get registry err")
					}
				}
				return PrintComponentListFromRegistry(registry, ioStreams, filter)
			}
			return PrintInstalledCompDef(c, ioStreams, filter)
		},
		Annotations: map[string]string{
			types.TagCommandType:  types.TypeExtension,
			types.TagCommandOrder: order,
		},
	}
	cmd.SetOut(ioStreams.Out)
	cmd.AddCommand(
		NewCompGetCommand(c, ioStreams),
	)
	cmd.Flags().BoolVar(&isDiscover, "discover", false, "discover packages in registries")
	cmd.PersistentFlags().StringVar(&regURL, "url", "", "specify the registry URL")
	cmd.PersistentFlags().StringVar(&regName, "registry", DefaultRegistry, "specify the registry name")
	cmd.PersistentFlags().StringVar(&token, "token", "", "specify token when using --url to specify registry url")
	cmd.Flags().StringVar(&label, types.LabelArg, "", "a label to filter components, the format is `--label type=terraform`")
	cmd.SetOut(ioStreams.Out)
	return cmd
}
