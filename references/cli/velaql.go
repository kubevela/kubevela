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

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
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

// NewQlCommand creates `ql` command for executing velaQL
func NewQlCommand(c common.Args, order string, ioStreams util.IOStreams) *cobra.Command {
	var cueFile, querySts string
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "ql",
		Short:   "Show result of executing velaQL.",
		Long:    "Show result of executing velaQL.",
		Example: `vela ql "<inner-view-name>{<param1>=<value1>,<param2>=<value2>}"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cueFile == "" && querySts == "" {
				return fmt.Errorf("please specify at least on VelaQL statement or velaql file path")
			}
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}
			if querySts != "" {
				return queryFromStatement(ctx, newClient, c, querySts, cmd)
			}
			return queryFromView(ctx, newClient, c, cueFile, cmd)
		},
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
	}
	cmd.Flags().StringVarP(&cueFile, "file", "f", "", "The CUE file path for VelaQL.")
	cmd.Flags().StringVarP(&querySts, "query", "q", "", "The query statement for VelaQL.")
	cmd.SetOut(ioStreams.Out)
	return cmd
}

// queryFromStatement print velaQL result from query statement with inner query view
func queryFromStatement(ctx context.Context, client client.Client, velaC common.Args, velaQLStatement string, cmd *cobra.Command) error {
	queryView, err := velaql.ParseVelaQL(velaQLStatement)
	if err != nil {
		return err
	}
	queryValue, err := QueryValue(ctx, client, velaC, &queryView)
	if err != nil {
		return err
	}
	return print(queryValue, cmd)
}

// queryFromView print velaQL result from query view
func queryFromView(ctx context.Context, client client.Client, velaC common.Args, velaQLViewPath string, cmd *cobra.Command) error {
	queryView, err := velaql.ParseVelaQLFromPath(velaQLViewPath)
	if err != nil {
		return err
	}
	queryValue, err := QueryValue(ctx, client, velaC, queryView)
	if err != nil {
		return err
	}
	return print(queryValue, cmd)
}

func print(queryValue *value.Value, cmd *cobra.Command) error {
	response, err := queryValue.CueValue().MarshalJSON()
	if err != nil {
		return err
	}
	var out bytes.Buffer
	err = json.Indent(&out, response, "", "    ")
	if err != nil {
		return err
	}
	cmd.Printf("%s\n", out.String())
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
func GetServiceEndpoints(ctx context.Context, client client.Client, appName string, namespace string, velaC common.Args, f Filter) ([]querytypes.ServiceEndpoint, error) {
	params := map[string]string{
		"appName": appName,
		"appNs":   namespace,
	}
	if f.Component != "" {
		params["name"] = f.Component
	}
	if f.Cluster != "" && f.ClusterNamespace != "" {
		params["cluster"] = f.Cluster
		params["clusterNs"] = f.ClusterNamespace
	}

	velaQL := MakeVelaQL("service-endpoints-view", params, "status")
	queryView, err := velaql.ParseVelaQL(velaQL)
	if err != nil {
		return nil, err
	}
	queryValue, err := QueryValue(ctx, client, velaC, &queryView)
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

// QueryValue get queryValue from velaQL
func QueryValue(ctx context.Context, client client.Client, velaC common.Args, queryView *velaql.QueryView) (*value.Value, error) {
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
	queryValue, err := velaql.NewViewHandler(client, config, dm, pd).QueryView(ctx, *queryView)
	if err != nil {
		return nil, err
	}
	return queryValue, nil
}
