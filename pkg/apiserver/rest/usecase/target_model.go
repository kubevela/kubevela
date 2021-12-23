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

package usecase

import (
	"context"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
)

func listTarget(ctx context.Context, ds datastore.DataStore, dsOption *datastore.ListOptions) ([]*model.Target, error) {
	if dsOption == nil {
		dsOption = &datastore.ListOptions{}
	}

	Target := model.Target{}
	Targets, err := ds.List(ctx, &Target, dsOption)
	if err != nil {
		log.Logger.Errorf("list target err %v", err)
		return nil, err
	}
	var respTargets []*model.Target
	for _, raw := range Targets {
		target, ok := raw.(*model.Target)
		if ok {
			respTargets = append(respTargets, target)
		}
	}
	return respTargets, nil
}
