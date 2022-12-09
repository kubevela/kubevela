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
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/pkg/velaql"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
)

// Filter filter options
type Filter struct {
	Component        string
	Cluster          string
	ClusterNamespace string
}

const (
	// ViewNamingRegex is a regex for names of view, essentially allowing something like `some-name-123`
	ViewNamingRegex = `^[a-z\d]+(-[a-z\d]+)*$`
)

// NewQlCommand creates `ql` command for executing velaQL
func NewQlCommand(c common.Args, order string, ioStreams util.IOStreams) *cobra.Command {
	var cueFile, querySts string
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:   "ql",
		Short: "Show result of executing velaQL.",
		Long: `Show result of executing velaQL, use it like:
		vela ql --query "inner-view-name{param1=value1,param2=value2}"
		vela ql --file ./ql.cue`,
		Example: `  Users can query with a query statement:
		vela ql --query "inner-view-name{param1=value1,param2=value2}"

  Query by a ql file:
		vela ql --file ./ql.cue
  Query by a ql file from remote url:
		vela ql --file https://my.host.to.cue/ql.cue
  Query by a ql file from stdin:
		cat ./ql.cue | vela ql --file -

Example content of ql.cue:
---
import (
	"vela/ql"
)
configmap: ql.#Read & {
   value: {
      kind: "ConfigMap"
      apiVersion: "v1"
      metadata: {
        name: "mycm"
      }
   }
}
status: configmap.value.data.key

export: "status"
---
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cueFile == "" && querySts == "" && len(args) == 0 {
				return fmt.Errorf("please specify at least one VelaQL statement or VelaQL file path")
			}

			if cueFile != "" {
				return queryFromView(ctx, c, cueFile, cmd)
			}
			if querySts == "" {
				// for compatibility
				querySts = args[0]
			}
			return queryFromStatement(ctx, c, querySts, cmd)
		},
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
	}
	cmd.Flags().StringVarP(&cueFile, "file", "f", "", "The CUE file path for VelaQL, it could be a remote url.")
	cmd.Flags().StringVarP(&querySts, "query", "q", "", "The query statement for VelaQL.")
	cmd.SetOut(ioStreams.Out)

	// Add subcommands like `create`, to `vela ql`
	cmd.AddCommand(NewQLApplyCommand(c))
	// TODO(charlie0129): add `vela ql delete` command to delete created views (ConfigMaps)
	// TODO(charlie0129): add `vela ql list` command to list user-created views (and views installed from addons, if that's feasible)

	return cmd
}

// NewQLApplyCommand creates a VelaQL view
func NewQLApplyCommand(c common.Args) *cobra.Command {
	var (
		viewFile string
	)
	cmd := &cobra.Command{
		Use:   "apply [view-name]",
		Short: "Create and store a VelaQL view",
		Long: `Create and store a VelaQL view to reuse it later.

You can specify your view file from:
	- a file (-f my-view.cue)
	- a URL (-f https://example.com/view.cue)
	- stdin (-f -)

View name can be automatically inferred from file/URL.
If we cannot infer a name from it, you must explicitly specify the view name (see examples).

If a view with the same name already exists, it will be updated.`,
		Example: `Assume your VelaQL view is stored in <my-view.cue>.

View name will be implicitly inferred from file name or URL (my-view):
	vela ql create -f my-view.cue

You can also explicitly specify view name (custom-name):
	vela ql create custom-name -f my-view.cue

If view name cannot be inferred, or you are reading from stdin (-f -), you must explicitly specify view name:
	cat my-view.cue | vela ql create custom-name -f -`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var viewName string

			if viewFile == "" {
				return fmt.Errorf("no cue file provided")
			}

			// If a view name is provided by the user,
			// we will use it instead of inferring from file name.
			if len(args) == 1 {
				viewName = args[0]
			} else if viewFile != "-" {
				// If the user doesn't provide a name, but a file/URL is provided,
				// try to get the file name of .cue file/URL provided.
				n, err := utils.GetFilenameFromLocalOrRemote(viewFile)
				if err != nil {
					return fmt.Errorf("cannot get filename from %s: %w", viewFile, err)
				}
				viewName = n
			}

			// In case we can't infer a view name from file/URL,
			// and the user didn't provide a view name,
			// we can't continue.
			if viewName == "" {
				return fmt.Errorf("no view name provided or cannot inferr view name from file")
			}

			// Just do some name checks, following a typical convention.
			// In case the inferred/user-provided name have some problems.
			re := regexp.MustCompile(ViewNamingRegex)
			if !re.MatchString(viewName) {
				return fmt.Errorf("view name should only cocntain lowercase letters, dashes, and numbers, but received: %s", viewName)
			}

			k8sClient, err := c.GetClient()
			if err != nil {
				return err
			}

			return velaql.StoreViewFromFile(context.Background(), k8sClient, viewFile, viewName)
		},
	}

	flag := cmd.Flags()
	flag.StringVarP(&viewFile, "file", "f", "", "CUE file that stores the view, can be local path, URL, or stdin (-)")

	return cmd
}

// queryFromStatement print velaQL result from query statement with inner query view
func queryFromStatement(ctx context.Context, velaC common.Args, velaQLStatement string, cmd *cobra.Command) error {
	queryView, err := velaql.ParseVelaQL(velaQLStatement)
	if err != nil {
		return err
	}
	queryValue, err := QueryValue(ctx, velaC, &queryView)
	if err != nil {
		return err
	}
	return printValue(queryValue, cmd)
}

// queryFromView print velaQL result from query view
func queryFromView(ctx context.Context, velaC common.Args, velaQLViewPath string, cmd *cobra.Command) error {
	queryView, err := velaql.ParseVelaQLFromPath(velaQLViewPath)
	if err != nil {
		return err
	}
	queryValue, err := QueryValue(ctx, velaC, queryView)
	if err != nil {
		return err
	}
	return printValue(queryValue, cmd)
}

func printValue(queryValue *value.Value, cmd *cobra.Command) error {
	response, err := queryValue.CueValue().MarshalJSON()
	if err != nil {
		return err
	}
	var out bytes.Buffer
	err = json.Indent(&out, response, "", "  ")
	if err != nil {
		return err
	}
	cmd.Println(strings.Trim(strings.TrimSpace(out.String()), "\""))
	return nil
}

// MakeVelaQL build velaQL
func MakeVelaQL(view string, params map[string]string, action string) string {
	var paramString string
	for k, v := range params {
		if paramString != "" {
			paramString = fmt.Sprintf("%s, %s=%s", paramString, k, v)
		} else {
			paramString = fmt.Sprintf("%s=%s", k, v)
		}
	}
	return fmt.Sprintf("%s{%s}.%s", view, paramString, action)
}

// GetServiceEndpoints get service endpoints by velaQL
func GetServiceEndpoints(ctx context.Context, appName string, namespace string, velaC common.Args, f Filter) ([]querytypes.ServiceEndpoint, error) {
	params := map[string]string{
		"appName": appName,
		"appNs":   namespace,
	}
	setFilterParams(f, params)

	velaQL := MakeVelaQL("service-endpoints-view", params, "status")
	queryView, err := velaql.ParseVelaQL(velaQL)
	if err != nil {
		return nil, err
	}
	queryValue, err := QueryValue(ctx, velaC, &queryView)
	if err != nil {
		return nil, err
	}
	var response = struct {
		Endpoints []querytypes.ServiceEndpoint `json:"endpoints"`
		Error     string                       `json:"error"`
	}{}
	if err := queryValue.UnmarshalTo(&response); err != nil {
		return nil, err
	}
	if response.Error != "" {
		return nil, fmt.Errorf(response.Error)
	}
	return response.Endpoints, nil
}

// GetApplicationPods get the pods by velaQL
func GetApplicationPods(ctx context.Context, appName string, namespace string, velaC common.Args, f Filter) ([]querytypes.PodBase, error) {
	params := map[string]string{
		"appName": appName,
		"appNs":   namespace,
	}
	setFilterParams(f, params)

	velaQL := MakeVelaQL("component-pod-view", params, "status")
	queryView, err := velaql.ParseVelaQL(velaQL)
	if err != nil {
		return nil, err
	}
	queryValue, err := QueryValue(ctx, velaC, &queryView)
	if err != nil {
		return nil, err
	}
	var response = struct {
		Pods  []querytypes.PodBase `json:"podList"`
		Error string               `json:"error"`
	}{}
	if err := queryValue.UnmarshalTo(&response); err != nil {
		return nil, err
	}
	if response.Error != "" {
		return nil, fmt.Errorf(response.Error)
	}
	return response.Pods, nil
}

// GetApplicationServices get the services by velaQL
func GetApplicationServices(ctx context.Context, appName string, namespace string, velaC common.Args, f Filter) ([]querytypes.ResourceItem, error) {
	params := map[string]string{
		"appName": appName,
		"appNs":   namespace,
	}
	setFilterParams(f, params)
	velaQL := MakeVelaQL("component-service-view", params, "status")
	queryView, err := velaql.ParseVelaQL(velaQL)
	if err != nil {
		return nil, err
	}
	queryValue, err := QueryValue(ctx, velaC, &queryView)
	if err != nil {
		return nil, err
	}
	var response = struct {
		Services []querytypes.ResourceItem `json:"services"`
		Error    string                    `json:"error"`
	}{}
	if err := queryValue.UnmarshalTo(&response); err != nil {
		return nil, err
	}
	if response.Error != "" {
		return nil, fmt.Errorf(response.Error)
	}
	return response.Services, nil
}

// setFilterParams will convert Filter fields to velaQL params
func setFilterParams(f Filter, params map[string]string) {
	if f.Component != "" {
		params["name"] = f.Component
	}
	if f.Cluster != "" {
		params["cluster"] = f.Cluster
	}
	if f.ClusterNamespace != "" {
		params["clusterNs"] = f.ClusterNamespace
	}

}

// QueryValue get queryValue from velaQL
func QueryValue(ctx context.Context, velaC common.Args, queryView *velaql.QueryView) (*value.Value, error) {
	dm, err := velaC.GetDiscoveryMapper()
	if err != nil {
		return nil, err
	}
	pd, err := velaC.GetPackageDiscover()
	if err != nil {
		return nil, err
	}
	config, err := velaC.GetConfig()
	if err != nil {
		return nil, err
	}
	client, err := velaC.GetClient()
	if err != nil {
		return nil, err
	}
	queryValue, err := velaql.NewViewHandler(client, config, dm, pd).QueryView(ctx, *queryView)
	if err != nil {
		return nil, err
	}
	return queryValue, nil
}
