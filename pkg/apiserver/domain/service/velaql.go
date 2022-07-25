/*
 Copyright 2021. The KubeVela Authors.

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

package service

import (
	"context"
	"encoding/base64"
	"strings"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/clients"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/velaql"
)

// VelaQLService velaQL service
type VelaQLService interface {
	QueryView(context.Context, string) (*apis.VelaQLViewResponse, error)
}

type velaQLServiceImpl struct {
	KubeClient client.Client `inject:"kubeClient"`
	KubeConfig *rest.Config  `inject:"kubeConfig"`
	dm         discoverymapper.DiscoveryMapper
	pd         *packages.PackageDiscover
}

// NewVelaQLService new velaQL service
func NewVelaQLService() VelaQLService {
	dm, err := clients.GetDiscoverMapper()
	if err != nil {
		log.Logger.Fatalf("get discover mapper failure %s", err.Error())
	}

	pd, err := clients.GetPackageDiscover()
	if err != nil {
		log.Logger.Fatalf("get package discover failure %s", err.Error())
	}
	return &velaQLServiceImpl{
		dm: dm,
		pd: pd,
	}
}

// QueryView get the view query results
func (v *velaQLServiceImpl) QueryView(ctx context.Context, velaQL string) (*apis.VelaQLViewResponse, error) {
	query, err := velaql.ParseVelaQL(velaQL)
	if err != nil {
		return nil, bcode.ErrParseVelaQL
	}

	queryValue, err := velaql.NewViewHandler(v.KubeClient, v.KubeConfig, v.dm, v.pd).QueryView(ctx, query)
	if err != nil {
		log.Logger.Errorf("fail to query the view %s", err.Error())
		return nil, bcode.ErrViewQuery
	}

	resp := apis.VelaQLViewResponse{}
	err = queryValue.UnmarshalTo(&resp)
	if err != nil {
		log.Logger.Errorf("decode the velaQL response to json failure %s", err.Error())
		return nil, bcode.ErrParseQuery2Json
	}
	if strings.Contains(velaQL, "collect-logs") {
		logs, ok := resp["logs"].(string)
		if ok {
			enc, _ := base64.StdEncoding.DecodeString(logs)
			resp["logs"] = string(enc)
		} else {
			resp["logs"] = ""
		}
	}
	return &resp, err
}
