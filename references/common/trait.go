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

package common

import (
	"context"

	client2 "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// ListRawWorkloadDefinitions will list raw definition
func ListRawWorkloadDefinitions(userNamespace string, c common.Args) ([]v1beta1.WorkloadDefinition, error) {
	client, err := c.GetClient()
	if err != nil {
		return nil, err
	}
	ctx := util.SetNamespaceInCtx(context.Background(), userNamespace)
	workloadList := v1beta1.WorkloadDefinitionList{}
	ns := ctx.Value(util.AppDefinitionNamespace).(string)
	if err = client.List(ctx, &workloadList, client2.InNamespace(ns)); err != nil {
		return nil, err
	}
	if ns == oam.SystemDefinitionNamespace {
		return workloadList.Items, nil
	}
	sysWorkloadList := v1beta1.WorkloadDefinitionList{}
	if err = client.List(ctx, &sysWorkloadList, client2.InNamespace(oam.SystemDefinitionNamespace)); err != nil {
		return nil, err
	}
	return append(workloadList.Items, sysWorkloadList.Items...), nil
}
