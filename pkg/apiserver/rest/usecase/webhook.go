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
	"errors"
	"fmt"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

// WebhookUsecase webhook usecase
type WebhookUsecase interface {
	HandleApplicationWebhook(ctx context.Context, token string, req apisv1.HandleApplicationWebhookRequest) (*apisv1.ApplicationDeployResponse, error)
}

type webhookUsecaseImpl struct {
	ds                 datastore.DataStore
	applicationUsecase ApplicationUsecase
}

// NewWebhookUsecase new webhook usecase
func NewWebhookUsecase(ds datastore.DataStore,
	applicationUsecase ApplicationUsecase,
) WebhookUsecase {
	return &webhookUsecaseImpl{
		ds:                 ds,
		applicationUsecase: applicationUsecase,
	}
}

func (c *webhookUsecaseImpl) HandleApplicationWebhook(ctx context.Context, token string, req apisv1.HandleApplicationWebhookRequest) (*apisv1.ApplicationDeployResponse, error) {
	webhookTrigger := &model.ApplicationTrigger{
		Token: token,
	}
	if err := c.ds.Get(ctx, webhookTrigger); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrInvalidWebhookToken
		}
		return nil, err
	}
	fmt.Println("===================", token)
	app := &model.Application{
		Name: webhookTrigger.AppPrimaryKey,
	}
	if err := c.ds.Get(ctx, app); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrApplicationNotExist
		}
		return nil, err
	}
	switch webhookTrigger.PayloadType {
	case model.PayloadTypeCustom:
		for comp, properties := range req.Upgrade {
			component := &model.ApplicationComponent{
				AppPrimaryKey: webhookTrigger.AppPrimaryKey,
				Name:          comp,
			}
			if err := c.ds.Get(ctx, component); err != nil {
				if errors.Is(err, datastore.ErrRecordNotExist) {
					return nil, bcode.ErrApplicationComponetNotExist
				}
				return nil, err
			}
			component.Properties = component.Properties.MergeFrom(*properties)
			if err := c.ds.Put(ctx, component); err != nil {
				return nil, err
			}
		}
		fmt.Println("=======", webhookTrigger.WorkflowName)
		return c.applicationUsecase.Deploy(ctx, app, apisv1.ApplicationDeployRequest{
			WorkflowName: webhookTrigger.WorkflowName,
			Note:         "triggered by webhook",
			TriggerType:  apisv1.TriggerTypeWebhook,
			Force:        true,
			CodeInfo:     req.CodeInfo,
		})
	default:
		return nil, bcode.ErrInvalidWebhookPayloadType
	}
}
