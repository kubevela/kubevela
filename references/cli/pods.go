/*
Copyright 2022 The KubeVela Authors.

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

	"github.com/gosuri/uitable"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/appfile"
)

func printAppPods(appName string, namespace string, f Filter, velaC common.Args) error {
	app, err := appfile.LoadApplication(namespace, appName, velaC)
	if err != nil {
		return err
	}
	var podArgs = PodArgs{
		Args:      velaC,
		Namespace: namespace,
		Filter:    f,
		App:       app,
	}

	table, err := podArgs.listPods(context.Background())
	if err != nil {
		return err
	}
	fmt.Println(table.String())
	return nil
}

// PodArgs creates arguments for `pods` command
type PodArgs struct {
	Args      common.Args
	Namespace string
	Filter    Filter
	App       *v1beta1.Application
}

func (p *PodArgs) listPods(ctx context.Context) (*uitable.Table, error) {
	pods, err := GetApplicationPods(ctx, p.App.Name, p.Namespace, p.Args, p.Filter)
	if err != nil {
		return nil, err
	}
	table := uitable.New()
	table.AddRow("CLUSTER", "COMPONENT", "POD NAME", "NAMESPACE", "PHASE", "CREATE TIME", "REVISION", "HOST")
	for _, pod := range pods {
		table.AddRow(pod.Cluster, pod.Component, pod.Metadata.Name, pod.Metadata.Namespace, pod.Status.Phase, pod.Metadata.CreationTime, pod.Metadata.Version.DeployVersion, pod.Status.NodeName)
	}
	return table, nil
}
