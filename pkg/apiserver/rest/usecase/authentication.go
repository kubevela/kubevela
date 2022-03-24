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
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/form3tech-oss/jwt-go"
	"golang.org/x/oauth2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

const (
	secretDexConfig    = "dex-config"
	secretDexConfigKey = "config.yaml"
	jwtIssuer          = "vela-issuer"
	signedKey          = "vela-singned"

	// GrantTypeAccess is the grant type for access token
	GrantTypeAccess = "access"
	// GrantTypeRefresh is the grant type for refresh token
	GrantTypeRefresh = "refresh"
)

// AuthenticationUsecase is the usecase of authentication
type AuthenticationUsecase interface {
	Login(ctx context.Context, loginReq apisv1.LoginRequest) (*apisv1.LoginResponse, error)
	RefreshToken(ctx context.Context, refreshToken string) (*apisv1.RefreshTokenResponse, error)
	GetDexConfig(ctx context.Context) (*apisv1.DexConfigResponse, error)
	GetLoginType(ctx context.Context) (*apisv1.GetLoginTypeResponse, error)
}

type authenticationUsecaseImpl struct {
	sysUsecase  SystemInfoUsecase
	userUsecase UserUsecase
	ds          datastore.DataStore
	kubeClient  client.Client
}

// NewAuthenticationUsecase new authentication usecase
func NewAuthenticationUsecase(ds datastore.DataStore, sysUsecase SystemInfoUsecase, userUsecase UserUsecase) AuthenticationUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("failed to get kube client: %s", err.Error())
	}
	return &authenticationUsecaseImpl{
		sysUsecase:  sysUsecase,
		userUsecase: userUsecase,
		ds:          ds,
		kubeClient:  kubecli,
	}
}

type authHandler interface {
	login(ctx context.Context) (*apisv1.UserBase, error)
}

type dexHandlerImpl struct {
	token   *oauth2.Token
	idToken *oidc.IDToken
	ds      datastore.DataStore
}

type localHandlerImpl struct {
	ds          datastore.DataStore
	userUsecase UserUsecase
	username    string
	password    string
}

func (a *authenticationUsecaseImpl) newDexHandler(ctx context.Context, req apisv1.LoginRequest) (*dexHandlerImpl, error) {
	if req.Code == "" {
		return nil, bcode.ErrInvalidLoginRequest
	}
	dexConfig, err := a.GetDexConfig(ctx)
	if err != nil {
		return nil, err
	}
	provider, err := oidc.NewProvider(ctx, dexConfig.Issuer)
	if err != nil {
		return nil, err
	}
	idTokenVerifier := provider.Verifier(&oidc.Config{ClientID: dexConfig.ClientID})
	oauth2Config := &oauth2.Config{
		ClientID:     dexConfig.ClientID,
		ClientSecret: dexConfig.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  dexConfig.RedirectURL,
	}
	oidcCtx := oidc.ClientContext(ctx, http.DefaultClient)
	token, err := oauth2Config.Exchange(oidcCtx, req.Code)
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

func (a *authenticationUsecaseImpl) newLocalHandler(req apisv1.LoginRequest) (*localHandlerImpl, error) {
	if req.Username == "" || req.Password == "" {
		return nil, bcode.ErrInvalidLoginRequest
	}
	return &localHandlerImpl{
		ds:          a.ds,
		userUsecase: a.userUsecase,
		username:    req.Username,
		password:    req.Password,
	}, nil
}

func (a *authenticationUsecaseImpl) Login(ctx context.Context, loginReq apisv1.LoginRequest) (*apisv1.LoginResponse, error) {
	var handler authHandler
	var err error
	sysInfo, err := a.sysUsecase.GetSystemInfo(ctx)
	if err != nil {
		return nil, err
	}
	loginType := sysInfo.LoginType

	switch loginType {
	case model.LoginTypeDex:
		handler, err = a.newDexHandler(ctx, loginReq)
		if err != nil {
			return nil, err
		}
	case model.LoginTypeLocal:
		handler, err = a.newLocalHandler(loginReq)
		if err != nil {
			return nil, err
		}
	default:
		return nil, bcode.ErrUnsupportedLoginType
	}
	userBase, err := handler.login(ctx)
	if err != nil {
		return nil, err
	}
	accessToken, err := a.generateJWTToken(userBase.Name, GrantTypeAccess, time.Hour)
	if err != nil {
		return nil, err
	}
	refreshToken, err := a.generateJWTToken(userBase.Name, GrantTypeRefresh, time.Hour*24)
	if err != nil {
		return nil, err
	}
	return &apisv1.LoginResponse{
		User:         userBase,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (a *authenticationUsecaseImpl) generateJWTToken(username, grantType string, expireDuration time.Duration) (string, error) {
	expire := time.Now().Add(expireDuration)
	claims := model.CustomClaims{
		StandardClaims: jwt.StandardClaims{
			NotBefore: time.Now().Unix(),
			ExpiresAt: expire.Unix(),
			Issuer:    jwtIssuer,
		},
		Username:  username,
		GrantType: grantType,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(signedKey))
}

func (a *authenticationUsecaseImpl) RefreshToken(ctx context.Context, refreshToken string) (*apisv1.RefreshTokenResponse, error) {
	claim, err := ParseToken(refreshToken)
	if err != nil {
		return nil, err
	}
	if claim.GrantType == GrantTypeRefresh {
		accessToken, err := a.generateJWTToken(claim.Username, GrantTypeAccess, time.Hour)
		if err != nil {
			return nil, err
		}
		// TODO: generate a new refresh token
		return &apisv1.RefreshTokenResponse{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
		}, nil
	}
	return nil, err
}

// ParseToken parses and verifies a token
func ParseToken(tokenString string) (*model.CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &model.CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(signedKey), nil
	})
	if err != nil {
		ve := &jwt.ValidationError{}
		if jwtErr := errors.As(err, ve); jwtErr {
			switch ve.Errors {
			case jwt.ValidationErrorExpired:
				return nil, bcode.ErrTokenExpired
			case jwt.ValidationErrorNotValidYet:
				return nil, bcode.ErrTokenNotValidYet
			case jwt.ValidationErrorMalformed:
				return nil, bcode.ErrTokenMalformed
			default:
				return nil, bcode.ErrTokenInvalid
			}
		}
		return nil, err
	}
	if claims, ok := token.Claims.(*model.CustomClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, bcode.ErrTokenInvalid
}

func (a *authenticationUsecaseImpl) GetDexConfig(ctx context.Context) (*apisv1.DexConfigResponse, error) {
	secret := &v1.Secret{}
	if err := a.kubeClient.Get(ctx, types.NamespacedName{
		Name:      secretDexConfig,
		Namespace: velatypes.DefaultKubeVelaNS,
	}, secret); err != nil {
		log.Logger.Errorf("failed to get dex config: %s", err.Error())
		return nil, err
	}
	var config struct {
		Issuer        string `json:"issuer"`
		StaticClients []struct {
			ID           string   `json:"id"`
			Secret       string   `json:"secret"`
			RedirectURIs []string `json:"redirectURIs"`
		} `json:"staticClients"`
	}

	if err := yaml.Unmarshal(secret.Data[secretDexConfigKey], &config); err != nil {
		log.Logger.Errorf("failed to unmarshal dex config: %s", err.Error())
		return nil, err
	}
	if len(config.StaticClients) < 1 || len(config.StaticClients[0].RedirectURIs) < 1 {
		return nil, fmt.Errorf("invalid dex config")
	}
	return &apisv1.DexConfigResponse{
		Issuer:       config.Issuer,
		ClientID:     config.StaticClients[0].ID,
		ClientSecret: config.StaticClients[0].Secret,
		RedirectURL:  config.StaticClients[0].RedirectURIs[0],
	}, nil
}

func (a *authenticationUsecaseImpl) GetLoginType(ctx context.Context) (*apisv1.GetLoginTypeResponse, error) {
	sysInfo, err := a.sysUsecase.GetSystemInfo(ctx)
	if err != nil {
		return nil, err
	}
	return &apisv1.GetLoginTypeResponse{
		LoginType: sysInfo.LoginType,
	}, nil
}

func (d *dexHandlerImpl) login(ctx context.Context) (*apisv1.UserBase, error) {
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
		}
		u.LastLoginTime = time.Now()
		if err := d.ds.Put(ctx, u); err != nil {
			return nil, err
		}
	} else if err := d.ds.Add(ctx, &model.User{
		Email:         claims.Email,
		Name:          claims.Name,
		LastLoginTime: time.Now(),
	}); err != nil {
		return nil, err
	}

	return &apisv1.UserBase{
		Name:  claims.Name,
		Email: claims.Email,
	}, nil
}

func (l *localHandlerImpl) login(ctx context.Context) (*apisv1.UserBase, error) {
	user, err := l.userUsecase.GetUser(ctx, l.username)
	if err != nil {
		return nil, err
	}
	if err := compareHashWithPassword(user.Password, l.password); err != nil {
		return nil, err
	}
	if err := l.userUsecase.updateUserLoginTime(ctx, user); err != nil {
		return nil, err
	}
	return &apisv1.UserBase{
		Name:  user.Name,
		Email: user.Email,
	}, nil
}
