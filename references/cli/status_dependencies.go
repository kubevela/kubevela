/*
Copyright 2026 The KubeVela Authors.

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
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/sources"
)

type dependenciesOutput struct {
	Name         string                       `json:"name"`
	Namespace    string                       `json:"namespace"`
	Dependencies []common.ComponentDependency `json:"dependencies"`
}

// printAppDependencies prints what each of the Application's components depends
// on, and why any component still waiting is waiting.
func printAppDependencies(cli client.Client, namespace, appName string, filter Filter, outputFormat string, out io.Writer) error {
	app, err := loadRemoteApplication(cli, namespace, appName)
	if err != nil {
		return err
	}
	deps := make([]common.ComponentDependency, 0, len(app.Status.Dependencies))
	for _, d := range app.Status.Dependencies {
		if filter.Component == "" || d.Component == filter.Component {
			deps = append(deps, d)
		}
	}

	if outputFormat != "" {
		str, err := printObj(outputFormat, dependenciesOutput{Name: appName, Namespace: namespace, Dependencies: deps})
		if err != nil {
			return err
		}
		_, err = out.Write([]byte(str))
		return err
	}

	if len(deps) == 0 {
		if filter.Component != "" {
			_, err := fmt.Fprintf(out, "Component %s of %s/%s has no dependencies.\n", filter.Component, namespace, appName)
			return err
		}
		_, err := fmt.Fprintf(out, "Application %s/%s has no component dependencies.\n", namespace, appName)
		return err
	}
	fmt.Fprintf(out, "Dependencies of %s/%s:\n\n", namespace, appName)
	table := tablewriter.NewWriter(out)
	table.SetHeader([]string{"COMPONENT", "DEPENDS ON", "SOURCE", "WHERE"})
	dependents := map[string]bool{}
	for _, d := range deps {
		dependents[d.Component] = true
		table.Append([]string{d.Component, d.DependsOn, string(d.Source), dependencyWhere(d)})
	}
	table.Render()

	for _, svc := range app.Status.Services {
		if !dependents[svc.Name] || svc.Healthy || svc.Message == "" {
			continue
		}
		name := svc.Name
		if svc.Cluster != "" {
			name += " (" + svc.Cluster + ")"
		}
		fmt.Fprintf(out, "\n%s: %s\n", name, svc.Message)
	}
	return nil
}

// dependencyWhere is where a dependency is read: beside the component, at the
// placement an expression names, or "-" for dependsOn and inputs, which say
// nothing of placement.
func dependencyWhere(d common.ComponentDependency) string {
	if d.Source != common.DependencySourceExpression {
		return "-"
	}
	if where := sources.DependencyPlacement(d); where != "" {
		return where
	}
	return "beside"
}
