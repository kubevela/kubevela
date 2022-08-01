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
	"fmt"
	"regexp"
	"text/template"
	"time"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/wercker/stern/stern"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
	"github.com/oam-dev/kubevela/references/appfile"
)

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
			app, err := appfile.LoadApplication(largs.Namespace, args[0], c)
			if err != nil {
				return err
			}
			largs.App = app
			ctx := context.Background()
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
	Namespace     string
	ContainerName string
	PodName       string
	ClusterName   string
	ComponentName string
	App           *v1beta1.Application
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
	pod, err := regexp.Compile(selectPod.Metadata.Name + ".*")
	if err != nil {
		return fmt.Errorf("fail to compile '%s' for logs query", selectPod.Metadata.Name+".*")
	}
	container := regexp.MustCompile(".*")
	if l.ContainerName != "" {
		container = regexp.MustCompile(l.ContainerName + ".*")
	}
	namespace := selectPod.Metadata.Namespace
	selector := labels.NewSelector()
	for k, v := range selectPod.Metadata.Labels {
		req, _ := labels.NewRequirement(k, selection.Equals, []string{v})
		if req != nil {
			selector = selector.Add(*req)
		}
	}

	config, err := l.Args.GetConfig()
	if err != nil {
		return err
	}
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	added, removed, err := stern.Watch(ctx,
		clientSet.CoreV1().Pods(namespace),
		pod,
		container,
		nil,
		[]stern.ContainerState{stern.RUNNING, stern.TERMINATED},
		selector,
	)
	if err != nil {
		return err
	}
	tails := make(map[string]*stern.Tail)
	logC := make(chan string, 1024)

	go func() {
		for {
			select {
			case str := <-logC:
				ioStreams.Infonln(str)
			case <-ctx.Done():
				return
			}
		}
	}()

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
	funs := map[string]interface{}{
		"json": func(in interface{}) (string, error) {
			b, err := json.Marshal(in)
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
		"color": func(color color.Color, text string) string {
			return color.SprintFunc()(text)
		},
	}
	template, err := template.New("log").Funcs(funs).Parse(t)
	if err != nil {
		return errors.Wrap(err, "unable to parse template")
	}

	go func() {
		for p := range added {
			id := p.GetID()
			if tails[id] != nil {
				continue
			}
			// 48h
			dur, _ := time.ParseDuration("48h")
			tail := stern.NewTail(p.Namespace, p.Pod, p.Container, template, &stern.TailOptions{
				Timestamps:   true,
				SinceSeconds: int64(dur.Seconds()),
				Exclude:      nil,
				Include:      nil,
				Namespace:    false,
				TailLines:    nil, // default for all logs
			})
			tails[id] = tail

			tail.Start(ctx, clientSet.CoreV1().Pods(p.Namespace), logC)
		}
	}()

	go func() {
		for p := range removed {
			id := p.GetID()
			if tails[id] == nil {
				continue
			}
			tails[id].Close()
			delete(tails, id)
		}
	}()

	<-ctx.Done()

	return nil
}
