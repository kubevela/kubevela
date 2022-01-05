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
	"time"

	"github.com/emicklei/go-restful/v3"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/policy/envbinding"
)

// WebhookUsecase webhook usecase
type WebhookUsecase interface {
	HandleApplicationWebhook(ctx context.Context, token string, req *restful.Request) (*apisv1.ApplicationDeployResponse, error)
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

func (c *webhookUsecaseImpl) HandleApplicationWebhook(ctx context.Context, token string, req *restful.Request) (*apisv1.ApplicationDeployResponse, error) {
	webhookTrigger := &model.ApplicationTrigger{
		Token: token,
	}
	if err := c.ds.Get(ctx, webhookTrigger); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrInvalidWebhookToken
		}
		return nil, err
	}
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
		return c.handleCustomWebhook(ctx, req, webhookTrigger, app)
	case model.PayloadTypeACR:
		return c.handleACRWebhook(ctx, req, webhookTrigger, app)
	default:
		return nil, bcode.ErrInvalidWebhookPayloadType
	}
}

func (c *webhookUsecaseImpl) handleCustomWebhook(ctx context.Context, req *restful.Request, webhookTrigger *model.ApplicationTrigger, app *model.Application) (*apisv1.ApplicationDeployResponse, error) {
	var webhookReq apisv1.HandleApplicationTriggerWebhookRequest
	if err := req.ReadEntity(&webhookReq); err != nil {
		return nil, bcode.ErrInvalidWebhookPayloadBody
	}
	for comp, properties := range webhookReq.Upgrade {
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
		merge, err := envbinding.MergeRawExtension(component.Properties.RawExtension(), properties.RawExtension())
		if err != nil {
			return nil, err
		}
		prop, err := model.NewJSONStructByStruct(merge)
		if err != nil {
			return nil, err
		}
		component.Properties = prop
		if err := c.ds.Put(ctx, component); err != nil {
			return nil, err
		}
	}
	return c.applicationUsecase.Deploy(ctx, app, apisv1.ApplicationDeployRequest{
		WorkflowName: webhookTrigger.WorkflowName,
		Note:         "triggered by webhook custom",
		TriggerType:  apisv1.TriggerTypeWebhook,
		Force:        true,
		CodeInfo:     webhookReq.CodeInfo,
	})
}

func (c *webhookUsecaseImpl) handleACRWebhook(ctx context.Context, req *restful.Request, webhookTrigger *model.ApplicationTrigger, app *model.Application) (*apisv1.ApplicationDeployResponse, error) {
	var acrReq apisv1.HandleApplicationTriggerACRRequest
	if err := req.ReadEntity(&acrReq); err != nil {
		return nil, bcode.ErrInvalidWebhookPayloadBody
	}
	if webhookTrigger.ComponentName == "" {
		return nil, bcode.ErrApplicationComponetNotExist
	}
	component := &model.ApplicationComponent{
		AppPrimaryKey: webhookTrigger.AppPrimaryKey,
		Name:          webhookTrigger.ComponentName,
	}
	if err := c.ds.Get(ctx, component); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrApplicationComponetNotExist
		}
		return nil, err
	}

	image := fmt.Sprintf("registry.%s.aliyuncs.com/%s:%s", acrReq.Repository.Region, acrReq.Repository.RepoFullName, acrReq.PushData.Tag)
	merge, err := envbinding.MergeRawExtension(component.Properties.RawExtension(), &runtime.RawExtension{
		Raw: []byte(fmt.Sprintf(`{"image": "%s"}`, image)),
	})
	if err != nil {
		return nil, err
	}
	prop, err := model.NewJSONStructByStruct(merge)
	if err != nil {
		return nil, err
	}
	component.Properties = prop
	if err := c.ds.Put(ctx, component); err != nil {
		return nil, err
	}

	return c.applicationUsecase.Deploy(ctx, app, apisv1.ApplicationDeployRequest{
		WorkflowName: webhookTrigger.WorkflowName,
		Note:         "triggered by webhook acr",
		TriggerType:  apisv1.TriggerTypeWebhook,
		Force:        true,
		ImageInfo: &model.ImageInfo{
			Type: model.PayloadTypeACR,
			Resource: &model.ImageResource{
				Digest:     acrReq.PushData.Digest,
				Tag:        acrReq.PushData.Tag,
				URL:        image,
				CreateTime: parseTimeString(acrReq.PushData.PushedAt),
			},
			Repository: &model.ImageRepository{
				Name:       acrReq.Repository.Name,
				Namespace:  acrReq.Repository.Namespace,
				FullName:   acrReq.Repository.RepoFullName,
				Region:     acrReq.Repository.Region,
				Type:       acrReq.Repository.RepoType,
				CreateTime: parseTimeString(acrReq.Repository.DateCreated),
			},
		},
	})
}

func parseTimeString(t string) time.Time {
	if t == "" {
		return time.Time{}
	}

	l, err := time.LoadLocation("PRC")
	if err != nil {
		log.Logger.Errorf("failed to load location: %v", err)
		return time.Time{}
	}
	parsedTime, err := time.ParseInLocation("2006-01-02 15:04:05", t, l)
	if err != nil {
		log.Logger.Errorf("failed to parse time: %v", err)
		return time.Time{}
	}
	return parsedTime
}
