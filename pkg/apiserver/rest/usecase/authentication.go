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
	"errors"

	"github.com/coreos/go-oidc"
	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

var (
	// DexIssuerURL is the dex issuer url
	DexIssuerURL = "https://kubevela-dex-issuer.com"
	// DexClientID is the dex client id
	DexClientID = "velaux"
)

const (
	dexLoginType   = "dex"
	localLoginType = "local"
)

// AuthenticationUsecase is the usecase of authentication
type AuthenticationUsecase interface {
	Login(ctx context.Context, req *restful.Request) (*apisv1.LoginResponse, error)
}

type authenticationUsecaseImpl struct {
	ds datastore.DataStore
}

// NewAuthenticationUsecase new authentication usecase
func NewAuthenticationUsecase(ds datastore.DataStore) AuthenticationUsecase {
	return &authenticationUsecaseImpl{
		ds: ds,
	}
}

type authHandler interface {
	login(ctx context.Context) (*apisv1.LoginResponse, error)
}

type dexHandlerImpl struct {
	idToken *oidc.IDToken
	ds      datastore.DataStore
}

func (a *authenticationUsecaseImpl) newDexHandler(ctx context.Context, req *restful.Request) (*dexHandlerImpl, error) {
	provider, err := oidc.NewProvider(ctx, DexIssuerURL)
	if err != nil {
		return nil, err
	}
	idTokenVerifier := provider.Verifier(&oidc.Config{ClientID: DexClientID})
	token := req.HeaderParameter("Authorization")
	idToken, err := idTokenVerifier.Verify(ctx, token)
	if err != nil {
		return nil, err
	}
	return &dexHandlerImpl{
		idToken: idToken,
		ds:      a.ds,
	}, nil
}

func (a *authenticationUsecaseImpl) Login(ctx context.Context, req *restful.Request) (*apisv1.LoginResponse, error) {
	var handler authHandler
	var err error
	loginType := req.QueryParameter("type")
	switch loginType {
	case dexLoginType:
		handler, err = a.newDexHandler(ctx, req)
		if err != nil {
			return nil, err
		}
	case localLoginType:
	default:
		return nil, bcode.ErrUnsupportedLoginType
	}
	return handler.login(ctx)
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
	if err := d.ds.Get(ctx, user); err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			if err := d.ds.Add(ctx, &model.User{
				Email: claims.Email,
				Name:  claims.Name,
			}); err != nil {
				return nil, err
			}
		}
	} else if user.Name != claims.Name {
		user.Name = claims.Name
		if err := d.ds.Put(ctx, user); err != nil {
			return nil, err
		}
	}
	return &apisv1.LoginResponse{
		UserInfo: apisv1.DetailUserResponse{
			Name:  claims.Name,
			Email: claims.Email,
		},
	}, nil
}
