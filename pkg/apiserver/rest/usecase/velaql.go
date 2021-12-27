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

package usecase

import (
	"context"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/velaql"
)

// VelaQLUsecase velaQL usecase
type VelaQLUsecase interface {
	QueryView(context.Context, string) (*apis.VelaQLViewResponse, error)
}

type velaQLUsecaseImpl struct {
	kubeClient client.Client
	kubeConfig *rest.Config
	dm         discoverymapper.DiscoveryMapper
	pd         *packages.PackageDiscover
}

// NewVelaQLUsecase new velaQL usecase
func NewVelaQLUsecase() VelaQLUsecase {
	k8sClient, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get kubeclient failure %s", err.Error())
	}

	kubeConfig, err := clients.GetKubeConfig()
	if err != nil {
		log.Logger.Fatalf("get kubeconfig failure %s", err.Error())
	}

	dm, err := clients.GetDiscoverMapper()
	if err != nil {
		log.Logger.Fatalf("get discover mapper failure %s", err.Error())
	}

	pd, err := clients.GetPackageDiscover()
	if err != nil {
		log.Logger.Fatalf("get package discover failure %s", err.Error())
	}
	return &velaQLUsecaseImpl{
		kubeClient: k8sClient,
		kubeConfig: kubeConfig,
		dm:         dm,
		pd:         pd,
	}
}

// QueryView get the view query results
func (v *velaQLUsecaseImpl) QueryView(ctx context.Context, velaQL string) (*apis.VelaQLViewResponse, error) {
	query, err := velaql.ParseVelaQL(velaQL)
	if err != nil {
		return nil, bcode.ErrParseVelaQL
	}

	queryValue, err := velaql.NewViewHandler(v.kubeClient, v.kubeConfig, v.dm, v.pd).QueryView(ctx, query)
	if err != nil {
		log.Logger.Errorf("fail to query the view %s", err.Error())
		return nil, bcode.ErrViewQuery
	}

	resp := apis.VelaQLViewResponse{}
	err = queryValue.UnmarshalTo(&resp)
	if err != nil {
		return nil, bcode.ErrParseQuery2Json
	}
	return &resp, err
}
