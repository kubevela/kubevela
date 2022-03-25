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
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"

	"github.com/oam-dev/kubevela/pkg/utils/common"
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
func NewQlCommand(c common.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "ql",
		Short:   "Show result of executing velaQL.",
		Long:    "Show result of executing velaQL.",
		Example: `vela ql "view{parameter=value1,parameter=value2}"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLength := len(args)
			if argsLength == 0 {
				return fmt.Errorf("please specify an VelaQL statement")
			}
			velaQL := args[0]
			println("%s", velaQL)
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}
			return printVelaQLResult(ctx, newClient, c, velaQL, cmd)
		},
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

// printVelaQLResult show velaQL result
func printVelaQLResult(ctx context.Context, client client.Client, velaC common.Args, velaQL string, cmd *cobra.Command) error {
	queryValue, err := QueryValue(ctx, client, velaC, velaQL)
	if err != nil {
		return err
	}
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
	for key, value := range params {
		if paramString != "" {
			paramString = fmt.Sprintf("%s, %s=%s", paramString, key, value)
		} else {
			paramString = fmt.Sprintf("%s=%s", key, value)
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
	queryValue, err := QueryValue(ctx, client, velaC, velaQL)
	if err != nil {
		return nil, err
	}
	var response = struct {
		Endpoints []querytypes.ServiceEndpoint `json:"endpoints"`
		Error     string                       `json:"error"`
	}{}
	if err := queryValue.CueValue().Decode(&response); err != nil {
		return nil, err
	}
	if response.Error != "" {
		return nil, fmt.Errorf(response.Error)
	}
	return response.Endpoints, nil
}

// QueryValue get queryValue from velaQL
func QueryValue(ctx context.Context, client client.Client, velaC common.Args, velaQL string) (*value.Value, error) {
	dm, err := velaC.GetDiscoveryMapper()
	if err != nil {
		return nil, err
	}
	pd, err := velaC.GetPackageDiscover()
	if err != nil {
		return nil, err
	}
	queryView, err := velaql.ParseVelaQL(velaQL)
	if err != nil {
		return nil, err
	}
	config, err := velaC.GetConfig()
	if err != nil {
		return nil, err
	}
	queryValue, err := velaql.NewViewHandler(client, config, dm, pd).QueryView(ctx, queryView)
	if err != nil {
		return nil, err
	}
	return queryValue, nil
}
