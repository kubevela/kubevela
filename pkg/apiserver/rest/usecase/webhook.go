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
	HandleApplicationWebhook(ctx context.Context, token string, req *restful.Request) (interface{}, error)
}

type webhookUsecaseImpl struct {
	ds                 datastore.DataStore
	applicationUsecase ApplicationUsecase
}

// WebhookHandlers is the webhook handlers
var WebhookHandlers []string

// NewWebhookUsecase new webhook usecase
func NewWebhookUsecase(ds datastore.DataStore,
	applicationUsecase ApplicationUsecase,
) WebhookUsecase {
	registerHandlers()
	return &webhookUsecaseImpl{
		ds:                 ds,
		applicationUsecase: applicationUsecase,
	}
}

func registerHandlers() {
	new(customHandlerImpl).install()
	new(acrHandlerImpl).install()
	new(harborHandlerImpl).install()
}

type webhookHandler interface {
	handle(ctx context.Context, trigger *model.ApplicationTrigger, app *model.Application) (interface{}, error)
	install()
}

type customHandlerImpl struct {
	req apisv1.HandleApplicationTriggerWebhookRequest
	w   *webhookUsecaseImpl
}

type acrHandlerImpl struct {
	req apisv1.HandleApplicationTriggerACRRequest
	w   *webhookUsecaseImpl
}

type dockerHubHandlerImpl struct {
	req apisv1.HandleApplicationTriggerDockerHubRequest
	w   *webhookUsecaseImpl
}

func (c *webhookUsecaseImpl) newCustomHandler(req *restful.Request) (webhookHandler, error) {
	var webhookReq apisv1.HandleApplicationTriggerWebhookRequest
	if err := req.ReadEntity(&webhookReq); err != nil {
		return nil, bcode.ErrInvalidWebhookPayloadBody
	}
	return &customHandlerImpl{
		req: webhookReq,
		w:   c,
	}, nil
}

func (c *webhookUsecaseImpl) newACRHandler(req *restful.Request) (webhookHandler, error) {
	var acrReq apisv1.HandleApplicationTriggerACRRequest
	if err := req.ReadEntity(&acrReq); err != nil {
		return nil, bcode.ErrInvalidWebhookPayloadBody
	}
	return &acrHandlerImpl{
		req: acrReq,
		w:   c,
	}, nil
}

func (c *webhookUsecaseImpl) newDockerHubHandler(req *restful.Request) (webhookHandler, error) {
	var dockerHubReq apisv1.HandleApplicationTriggerDockerHubRequest
	if err := req.ReadEntity(&dockerHubReq); err != nil {
		return nil, bcode.ErrInvalidWebhookPayloadBody
	}
	return &dockerHubHandlerImpl{
		req: dockerHubReq,
		w:   c,
	}, nil
}

func (c *webhookUsecaseImpl) HandleApplicationWebhook(ctx context.Context, token string, req *restful.Request) (interface{}, error) {
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

	var handler webhookHandler
	var err error
	switch webhookTrigger.PayloadType {
	case model.PayloadTypeCustom:
		handler, err = c.newCustomHandler(req)
		if err != nil {
			return nil, err
		}
	case model.PayloadTypeACR:
		handler, err = c.newACRHandler(req)
		if err != nil {
			return nil, err
		}
	case model.PayloadTypeHarbor:
		handler, err = c.newHarborHandler(req)
		if err != nil {
			return nil, err
		}
	case model.PayloadTypeDockerhub:
		handler, err = c.newDockerHubHandler(req)
		if err != nil {
			return nil, err
		}
	default:
		return nil, bcode.ErrInvalidWebhookPayloadType
	}

	return handler.handle(ctx, webhookTrigger, app)
}

func (c *webhookUsecaseImpl) patchComponentProperties(ctx context.Context, component *model.ApplicationComponent, patch *runtime.RawExtension) error {
	merge, err := envbinding.MergeRawExtension(component.Properties.RawExtension(), patch)
	if err != nil {
		return err
	}
	prop, err := model.NewJSONStructByStruct(merge)
	if err != nil {
		return err
	}
	component.Properties = prop
	if err := c.ds.Put(ctx, component); err != nil {
		return err
	}
	return nil
}

func (c *customHandlerImpl) handle(ctx context.Context, webhookTrigger *model.ApplicationTrigger, app *model.Application) (interface{}, error) {
	for comp, properties := range c.req.Upgrade {
		component := &model.ApplicationComponent{
			AppPrimaryKey: webhookTrigger.AppPrimaryKey,
			Name:          comp,
		}
		if err := c.w.ds.Get(ctx, component); err != nil {
			if errors.Is(err, datastore.ErrRecordNotExist) {
				return nil, bcode.ErrApplicationComponetNotExist
			}
			return nil, err
		}
		if err := c.w.patchComponentProperties(ctx, component, properties.RawExtension()); err != nil {
			return nil, err
		}
	}
	return c.w.applicationUsecase.Deploy(ctx, app, apisv1.ApplicationDeployRequest{
		WorkflowName: webhookTrigger.WorkflowName,
		Note:         "triggered by webhook custom",
		TriggerType:  apisv1.TriggerTypeWebhook,
		Force:        true,
		CodeInfo:     c.req.CodeInfo,
	})
}

func (c *customHandlerImpl) install() {
	WebhookHandlers = append(WebhookHandlers, model.PayloadTypeCustom)
}

func (c *acrHandlerImpl) handle(ctx context.Context, webhookTrigger *model.ApplicationTrigger, app *model.Application) (interface{}, error) {
	comp := &model.ApplicationComponent{
		AppPrimaryKey: webhookTrigger.AppPrimaryKey,
	}
	comps, err := c.w.ds.List(ctx, comp, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(comps) == 0 {
		return nil, bcode.ErrApplicationComponetNotExist
	}

	// use the first component as the target component
	component := comps[0].(*model.ApplicationComponent)
	acrReq := c.req
	image := fmt.Sprintf("registry.%s.aliyuncs.com/%s:%s", acrReq.Repository.Region, acrReq.Repository.RepoFullName, acrReq.PushData.Tag)
	if err := c.w.patchComponentProperties(ctx, component, &runtime.RawExtension{
		Raw: []byte(fmt.Sprintf(`{"image": "%s"}`, image)),
	}); err != nil {
		return nil, err
	}

	return c.w.applicationUsecase.Deploy(ctx, app, apisv1.ApplicationDeployRequest{
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

func (c *acrHandlerImpl) install() {
	WebhookHandlers = append(WebhookHandlers, model.PayloadTypeACR)
}

func (c dockerHubHandlerImpl) handle(ctx context.Context, trigger *model.ApplicationTrigger, app *model.Application) (interface{}, error) {
	dockerHubReq := c.req
	if dockerHubReq.Repository.Status != "Active" {
		log.Logger.Debugf("receive dockerhub webhook but not create event: %v", dockerHubReq)
		return &apisv1.ApplicationDockerhubWebhookResponse{
			State:       "failed",
			Description: "not create event",
		}, nil
	}

	comp := &model.ApplicationComponent{
		AppPrimaryKey: trigger.AppPrimaryKey,
	}
	comps, err := c.w.ds.List(ctx, comp, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(comps) == 0 {
		return nil, bcode.ErrApplicationComponetNotExist
	}

	// use the first component as the target component
	component := comps[0].(*model.ApplicationComponent)
	image := fmt.Sprintf("docker.io/%s:%s", dockerHubReq.Repository.RepoName, dockerHubReq.PushData.Tag)
	if err := c.w.patchComponentProperties(ctx, component, &runtime.RawExtension{
		Raw: []byte(fmt.Sprintf(`{"image": "%s"}`, image)),
	}); err != nil {
		return nil, err
	}

	repositoryType := "public"
	if dockerHubReq.Repository.IsPrivate {
		repositoryType = "private"
	}

	if _, err = c.w.applicationUsecase.Deploy(ctx, app, apisv1.ApplicationDeployRequest{
		WorkflowName: trigger.WorkflowName,
		Note:         "triggered by webhook dockerhub",
		TriggerType:  apisv1.TriggerTypeWebhook,
		Force:        true,
		ImageInfo: &model.ImageInfo{
			Type: model.PayloadTypeDockerhub,
			Resource: &model.ImageResource{
				Tag:        dockerHubReq.PushData.Tag,
				URL:        image,
				CreateTime: time.Unix(dockerHubReq.PushData.PushedAt, 0),
			},
			Repository: &model.ImageRepository{
				Name:       dockerHubReq.Repository.Name,
				Namespace:  dockerHubReq.Repository.Namespace,
				FullName:   dockerHubReq.Repository.RepoName,
				Type:       repositoryType,
				CreateTime: time.Unix(dockerHubReq.Repository.DateCreated, 0),
			},
		},
	}); err != nil {
		return nil, err
	}

	return &apisv1.ApplicationDockerhubWebhookResponse{
		State:       "success",
		Description: fmt.Sprintf("update application %s/%s success", app.Name, component.Name),
	}, nil
}

func (d dockerHubHandlerImpl) install() {
	WebhookHandlers = append(WebhookHandlers, model.PayloadTypeDockerhub)
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

type harborHandlerImpl struct {
	req apisv1.HandleApplicationHarborReq
	w   *webhookUsecaseImpl
}

func (c *webhookUsecaseImpl) newHarborHandler(req *restful.Request) (webhookHandler, error) {
	var harborReq apisv1.HandleApplicationHarborReq
	if err := req.ReadEntity(&harborReq); err != nil {
		return nil, bcode.ErrInvalidWebhookPayloadBody
	}
	if harborReq.Type != model.HarborEventTypePushArtifact {
		return nil, bcode.ErrInvalidWebhookPayloadBody
	}
	return &harborHandlerImpl{
		req: harborReq,
		w:   c,
	}, nil
}

func (c *harborHandlerImpl) install() {
	WebhookHandlers = append(WebhookHandlers, model.PayloadTypeHarbor)
}

func (c *harborHandlerImpl) handle(ctx context.Context, webhookTrigger *model.ApplicationTrigger, app *model.Application) (*apisv1.ApplicationDeployResponse, error) {
	resources := c.req.EventData.Resources
	if len(resources) < 1 {
		return nil, bcode.ErrInvalidWebhookPayloadBody
	}
	imageURL := resources[0].ResourceURL
	digest := resources[0].Digest
	tag := resources[0].Tag
	comp := &model.ApplicationComponent{
		AppPrimaryKey: webhookTrigger.AppPrimaryKey,
	}
	comps, err := c.w.ds.List(ctx, comp, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(comps) == 0 {
		return nil, bcode.ErrApplicationComponetNotExist
	}

	// use the first component as the target component
	component := comps[0].(*model.ApplicationComponent)
	harborReq := c.req
	if err := c.w.patchComponentProperties(ctx, component, &runtime.RawExtension{
		Raw: []byte(fmt.Sprintf(`{"image": "%s"}`, imageURL)),
	}); err != nil {
		return nil, err
	}
	return c.w.applicationUsecase.Deploy(ctx, app, apisv1.ApplicationDeployRequest{
		WorkflowName: webhookTrigger.WorkflowName,
		Note:         "triggered by webhook harbor",
		TriggerType:  apisv1.TriggerTypeWebhook,
		Force:        true,
		ImageInfo: &model.ImageInfo{
			Type: model.PayloadTypeHarbor,
			Resource: &model.ImageResource{
				Digest:     digest,
				Tag:        tag,
				URL:        imageURL,
				CreateTime: time.Unix(harborReq.OccurAt, 0),
			},
			Repository: &model.ImageRepository{
				Name:       harborReq.EventData.Repository.Name,
				Namespace:  harborReq.EventData.Repository.Namespace,
				FullName:   harborReq.EventData.Repository.RepoFullName,
				Type:       harborReq.EventData.Repository.RepoType,
				CreateTime: time.Unix(harborReq.EventData.Repository.DateCreated, 0),
			},
		},
	})
}
