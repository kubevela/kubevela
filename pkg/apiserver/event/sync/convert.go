/*
Copyright 2022 The KubeVela Authors.

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

package sync

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/event/sync/convert"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/workflow/step"
)

// ConvertApp2DatastoreApp will convert Application CR to datastore application related resources
func (c *CR2UX) ConvertApp2DatastoreApp(ctx context.Context, targetApp *v1beta1.Application) (*model.DataStoreApp, error) {
	cli := c.cli

	appName := c.getAppMetaName(ctx, targetApp.Name, targetApp.Namespace)

	project := model.DefaultInitName
	sourceOfTruth := model.FromCR
	if _, ok := targetApp.Labels[oam.LabelAddonName]; ok && strings.HasPrefix(targetApp.Name, "addon-") {
		project = model.DefaultAddonProject
		sourceOfTruth = model.FromInner
	}

	appMeta := &model.Application{
		Name:        appName,
		Description: model.AutoGenDesc,
		Alias:       targetApp.Name,
		Project:     project,
		Labels: map[string]string{
			model.LabelSyncNamespace:  targetApp.Namespace,
			model.LabelSyncGeneration: strconv.FormatInt(targetApp.Generation, 10),
			model.LabelSourceOfTruth:  sourceOfTruth,
		},
	}
	appMeta.CreateTime = targetApp.CreationTimestamp.Time
	appMeta.UpdateTime = time.Now()
	// 1. convert app meta and env
	dsApp := &model.DataStoreApp{
		AppMeta: appMeta,
	}

	// 1. convert the target

	existTarget := &model.Target{Project: project}
	existTargets, err := c.ds.List(ctx, existTarget, nil)
	if err != nil {
		return nil, fmt.Errorf("fail to list the targets, %w", err)
	}
	var envTargetNames []string
	dsApp.Targets, envTargetNames = convert.FromCRTargets(ctx, c.cli, targetApp, existTargets, project)

	// 2. convert the environment
	existEnv := &model.Env{Namespace: targetApp.Namespace, Project: project}
	existEnvs, err := c.ds.List(ctx, existEnv, nil)
	if err != nil {
		return nil, fmt.Errorf("fail to list the env, %w", err)
	}
	if len(existEnvs) > 0 {
		env := existEnvs[0].(*model.Env)
		for _, name := range envTargetNames {
			if !utils.StringsContain(env.Targets, name) {
				env.Targets = append(env.Targets, name)
			}
		}
		dsApp.Env = env
	}
	if dsApp.Env == nil {
		dsApp.Env = &model.Env{
			Name:        model.AutoGenEnvNamePrefix + targetApp.Namespace,
			Namespace:   targetApp.Namespace,
			Description: model.AutoGenDesc,
			Project:     project,
			Alias:       model.AutoGenEnvNamePrefix + targetApp.Namespace,
			Targets:     envTargetNames,
		}
	}
	dsApp.Eb = &model.EnvBinding{
		AppPrimaryKey: appMeta.PrimaryKey(),
		Name:          dsApp.Env.Name,
		AppDeployName: appMeta.GetAppNameForSynced(),
	}

	// 3. convert component and trait
	for _, cmp := range targetApp.Spec.Components {
		compModel, err := convert.FromCRComponent(appMeta.PrimaryKey(), cmp)
		if err != nil {
			return nil, err
		}
		dsApp.Comps = append(dsApp.Comps, &compModel)
	}

	// 4. convert workflow
	wf, steps, err := convert.FromCRWorkflow(ctx, cli, appMeta.PrimaryKey(), targetApp)
	if err != nil {
		return nil, err
	}
	wf.EnvName = dsApp.Env.Name
	dsApp.Workflow = &wf

	// 5. convert policy, some policies are references in workflow step, we need to sync all the outside policy to make that work
	var innerPlc = make(map[string]struct{})
	for _, plc := range targetApp.Spec.Policies {
		innerPlc[plc.Name] = struct{}{}
	}
	outsidePLC, err := step.LoadExternalPoliciesForWorkflow(ctx, cli, targetApp.Namespace, steps, targetApp.Spec.Policies)
	if err != nil {
		return nil, err
	}
	for _, plc := range outsidePLC {
		plcModel, err := convert.FromCRPolicy(appMeta.PrimaryKey(), plc, model.AutoGenRefPolicy)
		if _, ok := innerPlc[plc.Name]; ok {
			plcModel.Creator = model.AutoGenPolicy
		}
		if err != nil {
			return nil, err
		}
		plcModel.EnvName = dsApp.Env.Name
		dsApp.Policies = append(dsApp.Policies, &plcModel)
	}
	return dsApp, nil
}
