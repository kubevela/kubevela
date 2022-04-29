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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/form3tech-oss/jwt-go"
	"golang.org/x/oauth2"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

const (
	keyDex             = "dex"
	dexConfigName      = "dex-config"
	secretDexConfigKey = "config.yaml"
	dexAddonName       = "addon-dex"
	jwtIssuer          = "vela-issuer"

	// GrantTypeAccess is the grant type for access token
	GrantTypeAccess = "access"
	// GrantTypeRefresh is the grant type for refresh token
	GrantTypeRefresh = "refresh"
)

var signedKey = ""

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
	sysInfo, err := a.sysUsecase.Get(ctx)
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
	if userBase.Disabled {
		return nil, bcode.ErrUserAlreadyDisabled
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
		if errors.Is(err, bcode.ErrTokenExpired) {
			return nil, bcode.ErrRefreshTokenExpired
		}
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
		var ve *jwt.ValidationError
		if jwtErr := errors.As(err, &ve); jwtErr {
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
	config, err := getDexConfig(ctx, a.kubeClient)
	if err != nil {
		return nil, err
	}
	return &apisv1.DexConfigResponse{
		Issuer:       config.Issuer,
		ClientID:     config.StaticClients[0].ID,
		ClientSecret: config.StaticClients[0].Secret,
		RedirectURL:  config.StaticClients[0].RedirectURIs[0],
	}, nil
}

func generateDexConfig(ctx context.Context, kubeClient client.Client, update *model.UpdateDexConfig) error {
	secret, err := initDexConfig(ctx, kubeClient, update.VelaAddress)
	if err != nil {
		return err
	}
	dexConfig := &model.DexConfig{}
	if err := yaml.Unmarshal(secret.Data[secretDexConfigKey], dexConfig); err != nil {
		return err
	}
	if update.VelaAddress != "" {
		dexConfig.Issuer = fmt.Sprintf("%s/dex", update.VelaAddress)
		dexConfig.StaticClients[0].RedirectURIs = []string{fmt.Sprintf("%s/callback", update.VelaAddress)}
	}
	if update.Connectors != nil {
		dexConfig.Connectors = update.Connectors
	}
	if len(update.StaticPasswords) > 0 {
		dexConfig.StaticPasswords = update.StaticPasswords
	}
	config, err := model.NewJSONStructByStruct(dexConfig)
	if err != nil {
		return err
	}
	c, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(secret.Data[secretDexConfigKey], c) {
		secret.Data[secretDexConfigKey] = c
		if err := kubeClient.Update(ctx, secret); err != nil {
			return err
		}
		if err := restartDex(ctx, kubeClient); err != nil && !errors.Is(err, bcode.ErrDexNotFound) {
			return err
		}
	}
	return nil
}

func initDexConfig(ctx context.Context, kubeClient client.Client, velaAddress string) (*corev1.Secret, error) {
	dexConfig := model.DexConfig{
		Issuer: fmt.Sprintf("%s/dex", velaAddress),
		Web: model.DexWeb{
			HTTP: "0.0.0.0:5556",
		},
		Storage: model.DexStorage{
			Type: "kubernetes",
			Config: model.DexStorageConfig{
				InCluster: true,
			},
		},
		StaticClients: []model.DexStaticClient{
			{
				ID:           "velaux",
				Name:         "Vela UX",
				Secret:       "velaux-secret",
				RedirectURIs: []string{fmt.Sprintf("%s/callback", velaAddress)},
			},
		},
		EnablePasswordDB: true,
	}

	secret := &corev1.Secret{}
	if err := kubeClient.Get(ctx, types.NamespacedName{
		Name:      dexConfigName,
		Namespace: velatypes.DefaultKubeVelaNS,
	}, secret); err != nil || secret.Data == nil {
		if !kerrors.IsNotFound(err) {
			return nil, err
		}
		config, err := yaml.Marshal(dexConfig)
		if err != nil {
			return nil, err
		}
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dexConfigName,
				Namespace: velatypes.DefaultKubeVelaNS,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				secretDexConfigKey: config,
			},
		}
		if err := kubeClient.Create(ctx, secret); err != nil {
			return nil, err
		}
	}
	return secret, nil
}

func restartDex(ctx context.Context, kubeClient client.Client) error {
	dexApp := &v1beta1.Application{}
	if err := kubeClient.Get(ctx, types.NamespacedName{
		Name:      dexAddonName,
		Namespace: velatypes.DefaultKubeVelaNS,
	}, dexApp); err != nil {
		if kerrors.IsNotFound(err) {
			// skip restart dex if dex addon is not exist
			return nil
		}
		return err
	}
	for i, comp := range dexApp.Spec.Components {
		if comp.Name == keyDex {
			var v model.JSONStruct
			err := json.Unmarshal(comp.Properties.Raw, &v)
			if err != nil {
				return err
			}
			// restart the dex server
			if _, ok := v["values"]; ok {
				v["values"].(map[string]interface{})["env"] = map[string]string{
					"TIME_STAMP": time.Now().Format(time.RFC3339),
				}
			}
			dexApp.Spec.Components[i].Properties = v.RawExtension()
			if err := kubeClient.Update(ctx, dexApp); err != nil {
				return err
			}
			break
		}
	}

	return nil
}

func getDexConfig(ctx context.Context, kubeClient client.Client) (*model.DexConfig, error) {
	dexConfigSecret := &corev1.Secret{}
	if err := kubeClient.Get(ctx, types.NamespacedName{
		Name:      dexConfigName,
		Namespace: velatypes.DefaultKubeVelaNS,
	}, dexConfigSecret); err != nil {
		if kerrors.IsNotFound(err) {
			return nil, bcode.ErrDexConfigNotFound
		}
		return nil, err
	}
	if dexConfigSecret.Data == nil {
		return nil, bcode.ErrInvalidDexConfig
	}

	config := &model.DexConfig{}
	if err := yaml.Unmarshal(dexConfigSecret.Data[secretDexConfigKey], config); err != nil {
		log.Logger.Errorf("failed to unmarshal dex config: %s", err.Error())
		return nil, bcode.ErrInvalidDexConfig
	}
	if len(config.StaticClients) < 1 || len(config.StaticClients[0].RedirectURIs) < 1 {
		return nil, bcode.ErrInvalidDexConfig
	}
	return config, nil
}

func (a *authenticationUsecaseImpl) GetLoginType(ctx context.Context) (*apisv1.GetLoginTypeResponse, error) {
	sysInfo, err := a.sysUsecase.Get(ctx)
	if err != nil {
		return nil, err
	}
	loginType := sysInfo.LoginType
	if loginType == "" {
		loginType = model.LoginTypeLocal
	}
	return &apisv1.GetLoginTypeResponse{
		LoginType: loginType,
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
	userBase := &apisv1.UserBase{Email: claims.Email, Name: claims.Name}
	users, err := d.ds.List(ctx, user, &datastore.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(users) > 0 {
		u := users[0].(*model.User)
		u.LastLoginTime = time.Now()
		if err := d.ds.Put(ctx, u); err != nil {
			return nil, err
		}
		userBase.Name = u.Name
	} else if err := d.ds.Add(ctx, &model.User{
		Email:         claims.Email,
		Name:          claims.Name,
		LastLoginTime: time.Now(),
	}); err != nil {
		return nil, err
	}

	return userBase, nil
}

func (l *localHandlerImpl) login(ctx context.Context) (*apisv1.UserBase, error) {
	user, err := l.userUsecase.GetUser(ctx, l.username)
	if err != nil {
		if errors.Is(err, datastore.ErrRecordNotExist) {
			return nil, bcode.ErrUsernameNotExist
		}
		return nil, err
	}
	if err := compareHashWithPassword(user.Password, l.password); err != nil {
		return nil, err
	}
	if err := l.userUsecase.UpdateUserLoginTime(ctx, user); err != nil {
		return nil, err
	}
	return &apisv1.UserBase{
		Name:  user.Name,
		Email: user.Email,
	}, nil
}
