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

package usecase

import (
	"context"
	"net/http"
	"path"

	"github.com/coreos/go-oidc"
	"github.com/emicklei/go-restful/v3"
	"golang.org/x/oauth2"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/utils"
)

var dexConfig = apisv1.DexConfigResponse{}

func init() {
	dexConfig.ClientID = "velaux"
	dexConfig.ClientSecret = utils.RandomString(16)
	dexConfig.RedirectURL = "http://velaux.vela-system.svc.cluster.local/auth/callback"
	dexConfig.Issuer = path.Join(getVelaUXService(), "/dex")
}

// AuthenticationUsecase is the usecase of authentication
type AuthenticationUsecase interface {
	Login(ctx context.Context, req *restful.Request) (*apisv1.LoginResponse, error)
	GetDexConfig(ctx context.Context) *apisv1.DexConfigResponse
}

type authenticationUsecaseImpl struct {
	sysUsecase SystemInfoUsecase
	ds         datastore.DataStore
}

// NewAuthenticationUsecase new authentication usecase
func NewAuthenticationUsecase(ds datastore.DataStore, sysUsecase SystemInfoUsecase) AuthenticationUsecase {
	return &authenticationUsecaseImpl{
		sysUsecase: sysUsecase,
		ds:         ds,
	}
}

type authHandler interface {
	login(ctx context.Context) (*apisv1.LoginResponse, error)
}

type dexHandlerImpl struct {
	token   *oauth2.Token
	idToken *oidc.IDToken
	ds      datastore.DataStore
}

func (a *authenticationUsecaseImpl) newDexHandler(ctx context.Context, req *restful.Request) (*dexHandlerImpl, error) {
	provider, err := oidc.NewProvider(ctx, dexConfig.Issuer)
	if err != nil {
		return nil, err
	}
	idTokenVerifier := provider.Verifier(&oidc.Config{ClientID: dexConfig.ClientID})
	code := req.HeaderParameter("code")
	oauth2Config := &oauth2.Config{
		ClientID:     dexConfig.ClientID,
		ClientSecret: dexConfig.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  dexConfig.RedirectURL,
	}
	oidcCtx := oidc.ClientContext(ctx, http.DefaultClient)
	token, err := oauth2Config.Exchange(oidcCtx, code)
	if err != nil {
		return nil, err
	}
	idToken, err := idTokenVerifier.Verify(ctx, token.Extra("id_token").(string))
	if err != nil {
		return nil, err
	}
	return &dexHandlerImpl{
		token:   token,
		idToken: idToken,
		ds:      a.ds,
	}, nil
}

func (a *authenticationUsecaseImpl) Login(ctx context.Context, req *restful.Request) (*apisv1.LoginResponse, error) {
	var handler authHandler
	var err error
	sysInfo, err := a.sysUsecase.GetSystemInfo(ctx)
	if err != nil {
		return nil, err
	}
	loginType := sysInfo.LoginType

	switch loginType {
	case model.LoginTypeDex:
		handler, err = a.newDexHandler(ctx, req)
		if err != nil {
			return nil, err
		}
	case model.LoginTypeLocal:
	default:
		return nil, bcode.ErrUnsupportedLoginType
	}
	return handler.login(ctx)
}

func (a *authenticationUsecaseImpl) GetDexConfig(ctx context.Context) *apisv1.DexConfigResponse {
	return &dexConfig
}

func (d *dexHandlerImpl) login(ctx context.Context) (*apisv1.LoginResponse, error) {
	var claims struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := d.idToken.Claims(&claims); err != nil {
		return nil, err
	}

	user := &model.User{Email: claims.Email}
	users, err := d.ds.List(ctx, user, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(users) > 0 {
		u := users[0].(*model.User)
		if u.Name != claims.Name {
			u.Name = claims.Name
			if err := d.ds.Put(ctx, u); err != nil {
				return nil, err
			}
		}
	} else if err := d.ds.Add(ctx, &model.User{
		Email: claims.Email,
		Name:  claims.Name,
	}); err != nil {
		return nil, err
	}

	return &apisv1.LoginResponse{
		UserInfo: apisv1.DetailUserResponse{
			Name:  claims.Name,
			Email: claims.Email,
		},
		AccessToken:  d.token.AccessToken,
		RefreshToken: d.token.RefreshToken,
	}, nil
}

// TODO(fog)
func getVelaUXService() string {
	return ""
}
