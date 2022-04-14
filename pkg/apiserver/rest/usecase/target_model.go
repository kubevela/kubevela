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

	apierror "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils"
	velaerr "github.com/oam-dev/kubevela/pkg/utils/errors"
)

func createTargetNamespace(ctx context.Context, k8sClient client.Client, clusterName, namespace, targetName string) error {
	if clusterName == "" || namespace == "" {
		return bcode.ErrTargetInvalidWithEmptyClusterOrNamespace
	}
	err := utils.CreateOrUpdateNamespace(multicluster.ContextWithClusterName(ctx, clusterName), k8sClient, namespace, utils.MergeOverrideLabels(map[string]string{
		oam.LabelRuntimeNamespaceUsage: oam.VelaNamespaceUsageTarget,
	}), utils.MergeNoConflictLabels(map[string]string{
		oam.LabelNamespaceOfTargetName: targetName,
	}))
	if velaerr.IsLabelConflict(err) {
		log.Logger.Errorf("update namespace for target err %v", err)
		return bcode.ErrTargetNamespaceAlreadyBound
	}
	if err != nil {
		return err
	}
	return nil
}

func deleteTargetNamespace(ctx context.Context, k8sClient client.Client, clusterName, namespace, targetName string) error {
	err := utils.UpdateNamespace(multicluster.ContextWithClusterName(ctx, clusterName), k8sClient, namespace,
		// check no conflict label first to make sure the namespace belong to the target, then override it
		utils.MergeNoConflictLabels(map[string]string{
			oam.LabelNamespaceOfTargetName: targetName,
		}),
		utils.MergeOverrideLabels(map[string]string{
			oam.LabelRuntimeNamespaceUsage: "",
			oam.LabelNamespaceOfTargetName: "",
		}))
	if apierror.IsNotFound(err) {
		return nil
	}
	return err
}

func createTarget(ctx context.Context, ds datastore.DataStore, tg *model.Target) error {
	// check Target name.
	exit, err := ds.IsExist(ctx, tg)
	if err != nil {
		log.Logger.Errorf("check target existence failure %s", err.Error())
		return err
	}
	if exit {
		return bcode.ErrTargetExist
	}

	if err = ds.Add(ctx, tg); err != nil {
		log.Logger.Errorf("add target failure %s", err.Error())
		return err
	}
	return nil
}

func listTarget(ctx context.Context, ds datastore.DataStore, project string, dsOption *datastore.ListOptions) ([]*model.Target, error) {
	if dsOption == nil {
		dsOption = &datastore.ListOptions{}
	}
	target := model.Target{}
	if project != "" {
		target.Project = project
	}
	Targets, err := ds.List(ctx, &target, dsOption)
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
