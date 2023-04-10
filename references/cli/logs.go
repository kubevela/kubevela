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
	"fmt"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
	"github.com/oam-dev/kubevela/references/appfile"
)

var re = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)

// NewLogsCommand creates `logs` command to tail logs of application
func NewLogsCommand(c common.Args, order string, ioStreams util.IOStreams) *cobra.Command {
	largs := &Args{Args: c}
	cmd := &cobra.Command{
		Use:   "logs APP_NAME",
		Short: "Tail logs for application.",
		Long:  "Tail logs for vela application.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			largs.Namespace, err = GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			largs.Name = args[0]
			ctx := context.Background()
			app, err := appfile.LoadApplication(largs.Namespace, args[0], c)
			if err != nil {
				return err
			}
			largs.App = app
			if err := largs.Run(ctx, ioStreams); err != nil {
				return err
			}
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
	}

	cmd.Flags().StringVarP(&largs.Output, "output", "o", "default", "output format for logs, support: [default, raw, json]")
	cmd.Flags().StringVarP(&largs.ComponentName, "component", "c", "", "filter the pod by the component name")
	cmd.Flags().StringVarP(&largs.ClusterName, "cluster", "", "", "filter the pod by the cluster name")
	cmd.Flags().StringVarP(&largs.PodName, "pod", "p", "", "specify the pod name")
	cmd.Flags().StringVarP(&largs.ContainerName, "container", "", "", "specify the container name")
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// Args creates arguments for `logs` command
type Args struct {
	Output        string
	Args          common.Args
	Name          string
	CtxName       string
	Namespace     string
	ContainerName string
	PodName       string
	ClusterName   string
	ComponentName string
	StepName      string
	App           *v1beta1.Application
}

func (l *Args) printPodLogs(ctx context.Context, ioStreams util.IOStreams, selectPod *querytypes.PodBase, filters []string) error {
	config, err := l.Args.GetConfig()
	if err != nil {
		return err
	}
	logC := make(chan string, 1024)

	var t string
	switch l.Output {
	case "default":
		if color.NoColor {
			t = "{{.ContainerName}} {{.Message}}"
		} else {
			t = "{{color .ContainerColor .ContainerName}} {{.Message}}"
		}
	case "raw":
		t = "{{.Message}}"
	case "json":
		t = "{{json .}}\n"
	}
	go func() {
		for {
			select {
			case str := <-logC:
				show := true
				for _, filter := range filters {
					if !strings.Contains(str, filter) {
						show = false
						break
					}
				}
				if show {
					match := re.FindStringSubmatch(str)
					if len(match) > 1 {
						str = strings.ReplaceAll(match[1], "\\n", "\n")
					}
					ioStreams.Infonln(str)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	err = utils.GetPodsLogs(ctx, config, l.ContainerName, []*querytypes.PodBase{selectPod}, t, logC, nil)
	if err != nil {
		return err
	}

	return nil
}

// Run refer to the implementation at https://github.com/oam-dev/stern/blob/master/stern/main.go
func (l *Args) Run(ctx context.Context, ioStreams util.IOStreams) error {
	pods, err := GetApplicationPods(ctx, l.App.Name, l.App.Namespace, l.Args, Filter{
		Component: l.ComponentName,
		Cluster:   l.ClusterName,
	})
	if err != nil {
		return err
	}
	var selectPod *querytypes.PodBase
	if l.PodName != "" {
		for i, pod := range pods {
			if pod.Metadata.Name == l.PodName {
				selectPod = &pods[i]
				break
			}
		}
		if selectPod == nil {
			fmt.Println("The Pod you specified does not exist, please select it from the list.")
		}
	}
	if selectPod == nil {
		selectPod, err = AskToChooseOnePod(pods)
		if err != nil {
			return err
		}
	}

	if selectPod == nil {
		return nil
	}

	if selectPod.Cluster != "" {
		ctx = multicluster.ContextWithClusterName(ctx, selectPod.Cluster)
	}
	return l.printPodLogs(ctx, ioStreams, selectPod, nil)
}
