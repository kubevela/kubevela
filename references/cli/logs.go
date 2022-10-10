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
	"io"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/wercker/stern/stern"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	wfTypes "github.com/kubevela/workflow/pkg/types"
	wfUtils "github.com/kubevela/workflow/pkg/utils"

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
			largs.Name = args[0]
			ctx := context.Background()
			if largs.StepName != "" {
				cli, err := c.GetClient()
				if err != nil {
					return err
				}
				ctxName, label, err := getContextFromInstance(ctx, cli, largs.Namespace, largs.Name)
				if err != nil {
					return err
				}
				largs.CtxName = ctxName
				return largs.printStepLogs(ctx, ioStreams, label)
			}
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
	cmd.Flags().StringVarP(&largs.StepName, "step", "s", "", "specify the step name")
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

func (l *Args) printStepLogs(ctx context.Context, ioStreams util.IOStreams, label map[string]string) error {
	cli, err := l.Args.GetClient()
	if err != nil {
		return err
	}
	logConfig, err := wfUtils.GetLogConfigFromStep(ctx, cli, l.CtxName, l.Name, l.Namespace, l.StepName)
	if err != nil {
		return err
	}
	var source string
	if logConfig.Data && logConfig.Source != nil {
		prompt := &survey.Select{
			Message: "Select logs from data or source",
			Options: []string{"data", "source"},
		}
		err := survey.AskOne(prompt, &source, survey.WithValidator(survey.Required))
		if err != nil {
			return fmt.Errorf("failed to select %s: %w", source, err)
		}
		if source != "data" {
			logConfig.Data = false
		}
	}
	if logConfig.Data {
		return l.printResourceLogs(ctx, cli, ioStreams, []wfTypes.Resource{{
			Namespace:     "vela-system",
			LabelSelector: label,
		}}, []string{fmt.Sprintf(`step_name="%s"`, l.StepName), fmt.Sprintf("%s/%s", l.Namespace, l.Name)})
	}
	if logConfig.Source != nil {
		if len(logConfig.Source.Resources) > 0 {
			return l.printResourceLogs(ctx, cli, ioStreams, logConfig.Source.Resources, nil)
		}
		if logConfig.Source.URL != "" {
			readCloser, err := wfUtils.GetLogsFromURL(ctx, logConfig.Source.URL)
			if err != nil {
				return err
			}
			//nolint:errcheck
			defer readCloser.Close()
			if _, err := io.Copy(ioStreams.Out, readCloser); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *Args) printResourceLogs(ctx context.Context, cli client.Client, ioStreams util.IOStreams, resources []wfTypes.Resource, filters []string) error {
	pods, err := wfUtils.GetPodListFromResources(ctx, cli, resources)
	if err != nil {
		return err
	}
	podList := make([]querytypes.PodBase, 0)
	for _, pod := range pods {
		podBase := querytypes.PodBase{}
		podBase.Metadata.Name = pod.Name
		podBase.Metadata.Namespace = pod.Namespace
		podList = append(podList, podBase)
	}
	if len(pods) == 0 {
		return errors.New("no pod found")
	}
	var selectPod *querytypes.PodBase
	if len(pods) > 1 {
		selectPod, err = AskToChooseOnePod(podList)
		if err != nil {
			return err
		}
	} else {
		selectPod = &podList[0]
	}
	return l.printPodLogs(ctx, ioStreams, selectPod, filters)
}

func (l *Args) printPodLogs(ctx context.Context, ioStreams util.IOStreams, selectPod *querytypes.PodBase, filters []string) error {
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
				show := true
				for _, filter := range filters {
					if !strings.Contains(str, filter) {
						show = false
						break
					}
				}
				if show {
					ioStreams.Infonln(str)
				}
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

func getContextFromInstance(ctx context.Context, cli client.Client, namespace, name string) (string, map[string]string, error) {
	app := &v1beta1.Application{}
	if err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, app); err == nil {
		if app.Status.Workflow != nil && app.Status.Workflow.ContextBackend != nil {
			return app.Status.Workflow.ContextBackend.Name, map[string]string{"app.kubernetes.io/name": "vela-core"}, nil
		}
		return "", nil, fmt.Errorf("no context found in application %s", name)
	}
	wr := &workflowv1alpha1.WorkflowRun{}
	if err := cli.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, wr); err == nil {
		if wr.Status.ContextBackend != nil {
			return wr.Status.ContextBackend.Name, map[string]string{"app.kubernetes.io/name": "vela-workflow"}, nil
		}
		return "", nil, fmt.Errorf("no context found in workflowrun %s", name)
	}
	return "", nil, fmt.Errorf("no context found in application %s", name)
}
