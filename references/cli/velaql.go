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

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/velaql"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
)

// GetServiceEndpoints get service endpoints by velaQL
func GetServiceEndpoints(ctx context.Context, client client.Client, appName string, namespace string, velaC common.Args) ([]querytypes.ServiceEndpoint, error) {
	dm, err := velaC.GetDiscoveryMapper()
	if err != nil {
		return nil, err
	}
	pd, err := velaC.GetPackageDiscover()
	if err != nil {
		return nil, err
	}
	queryView, err := velaql.ParseVelaQL(fmt.Sprintf("service-endpoints-view{appName=%s,appNs=%s}.status", appName, namespace))
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
