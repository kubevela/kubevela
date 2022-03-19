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

	"golang.org/x/crypto/bcrypt"
	"helm.sh/helm/v3/pkg/time"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

// UserUsecase User manage api
type UserUsecase interface {
	GetUser(ctx context.Context, username string) (*model.User, error)
	DetailUser(ctx context.Context, user *model.User) (*apisv1.DetailUserResponse, error)
	DeleteUser(ctx context.Context, username string) error
	CreateUser(ctx context.Context, req apisv1.CreateUserRequest) (*apisv1.UserBase, error)
	UpdateUser(ctx context.Context, user *model.User, req apisv1.UpdateUserRequest) (*apisv1.UserBase, error)
	ListUsers(ctx context.Context, page, pageSize int, listOptions apisv1.ListUserOptions) (*apisv1.ListUserResponse, error)
	DisableUser(ctx context.Context, user *model.User) error
	EnableUser(ctx context.Context, user *model.User) error
	updateUserLoginTime(ctx context.Context, user *model.User) error
}

type userUsecaseImpl struct {
	ds             datastore.DataStore
	k8sClient      client.Client
	projectUsecase ProjectUsecase
	sysUsecase     SystemInfoUsecase
}

// NewUserUsecase new User usecase
func NewUserUsecase(ds datastore.DataStore, projectUsecase ProjectUsecase, sysUsecase SystemInfoUsecase) UserUsecase {
	k8sClient, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get k8sClient failure: %s", err.Error())
	}
	return &userUsecaseImpl{
		k8sClient:      k8sClient,
		ds:             ds,
		projectUsecase: projectUsecase,
		sysUsecase:     sysUsecase,
	}
}

// GetUser get user
func (u *userUsecaseImpl) GetUser(ctx context.Context, username string) (*model.User, error) {
	user := &model.User{
		Name: username,
	}
	if err := u.ds.Get(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// DetailUser return user detail
func (u *userUsecaseImpl) DetailUser(ctx context.Context, user *model.User) (*apisv1.DetailUserResponse, error) {
	detailUser := convertUserModel(user)
	pUser := &model.ProjectUser{
		Username: user.Name,
	}
	projectUsers, err := u.ds.List(ctx, pUser, &datastore.ListOptions{
		SortBy: []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}},
	})
	if err != nil {
		return nil, err
	}
	for _, v := range projectUsers {
		pu, ok := v.(*model.ProjectUser)
		if ok {
			project, err := u.projectUsecase.GetProject(ctx, pu.ProjectName)
			if err != nil {
				log.Logger.Errorf("failed to delete project(%s) info: %s", pu.ProjectName, err.Error())
				continue
			}
			detailUser.Projects = append(detailUser.Projects, apisv1.ProjectUserBase{
				Name:      pu.ProjectName,
				Alias:     project.Alias,
				UserRoles: pu.UserRoles,
			})
		}
	}
	return detailUser, nil
}

// DeleteUser delete user
func (u *userUsecaseImpl) DeleteUser(ctx context.Context, username string) error {
	pUser := &model.ProjectUser{
		Username: username,
	}

	projectUsers, err := u.ds.List(ctx, pUser, &datastore.ListOptions{})
	if err != nil {
		return err
	}
	for _, v := range projectUsers {
		pu := v.(*model.ProjectUser)
		if err := u.ds.Delete(ctx, pu); err != nil {
			log.Logger.Errorf("failed to delete project user %s: %s", pu.PrimaryKey(), err.Error())
		}
	}
	if err := u.ds.Delete(ctx, &model.User{Name: username}); err != nil {
		log.Logger.Errorf("failed to delete user", username, err.Error())
		return err
	}
	return nil
}

// CreateUser create user
func (u *userUsecaseImpl) CreateUser(ctx context.Context, req apisv1.CreateUserRequest) (*apisv1.UserBase, error) {
	sysInfo, err := u.sysUsecase.GetSystemInfo(ctx)
	if err != nil {
		return nil, err
	}
	if sysInfo.LoginType == model.LoginTypeDex {
		return nil, bcode.ErrUserCannotModified
	}
	hash, err := GeneratePasswordHash(req.Password)
	if err != nil {
		return nil, err
	}
	user := &model.User{
		Name:     req.Name,
		Alias:    req.Alias,
		Email:    req.Email,
		Password: hash,
		Disabled: false,
	}
	if err := u.ds.Add(ctx, user); err != nil {
		return nil, err
	}
	return convertUserBase(user), nil
}

// UpdateUser update user
func (u *userUsecaseImpl) UpdateUser(ctx context.Context, user *model.User, req apisv1.UpdateUserRequest) (*apisv1.UserBase, error) {
	sysInfo, err := u.sysUsecase.GetSystemInfo(ctx)
	if err != nil {
		return nil, err
	}
	if sysInfo.LoginType == model.LoginTypeDex {
		return nil, bcode.ErrUserCannotModified
	}
	if req.Alias != "" {
		user.Alias = req.Alias
	}
	if req.Password != "" {
		hash, err := GeneratePasswordHash(req.Password)
		if err != nil {
			return nil, err
		}
		user.Password = hash
	}
	if req.Email != "" {
		if user.Email != "" {
			return nil, bcode.ErrUnsupportedEmailModification
		}
		user.Email = req.Email
	}
	if err := u.ds.Put(ctx, user); err != nil {
		return nil, err
	}
	return convertUserBase(user), nil
}

// ListUsers list users
func (u *userUsecaseImpl) ListUsers(ctx context.Context, page, pageSize int, listOptions apisv1.ListUserOptions) (*apisv1.ListUserResponse, error) {
	user := &model.User{}
	var queries []datastore.FuzzyQueryOption
	if listOptions.Name != "" {
		queries = append(queries, datastore.FuzzyQueryOption{Key: "name", Query: listOptions.Name})
	}
	if listOptions.Email != "" {
		queries = append(queries, datastore.FuzzyQueryOption{Key: "email", Query: listOptions.Email})
	}
	if listOptions.Alias != "" {
		queries = append(queries, datastore.FuzzyQueryOption{Key: "alias", Query: listOptions.Alias})
	}
	fo := datastore.FilterOptions{Queries: queries}

	var userList []*apisv1.DetailUserResponse
	users, err := u.ds.List(ctx, user, &datastore.ListOptions{
		Page:          page,
		PageSize:      pageSize,
		SortBy:        []datastore.SortOption{{Key: "createTime", Order: datastore.SortOrderDescending}},
		FilterOptions: fo,
	})
	if err != nil {
		return nil, err
	}
	for _, v := range users {
		user, ok := v.(*model.User)
		if ok {
			userList = append(userList, convertUserModel(user))
		}
	}
	count, err := u.ds.Count(ctx, user, &fo)
	if err != nil {
		return nil, err
	}

	return &apisv1.ListUserResponse{
		Users: userList,
		Total: count,
	}, nil
}

// DisableUser disable user
func (u *userUsecaseImpl) DisableUser(ctx context.Context, user *model.User) error {
	if user.Disabled {
		return bcode.ErrUserAlreadyDisabled
	}
	user.Disabled = true
	return u.ds.Put(ctx, user)
}

// EnableUser disable user
func (u *userUsecaseImpl) EnableUser(ctx context.Context, user *model.User) error {
	if !user.Disabled {
		return bcode.ErrUserAlreadyEnabled
	}
	user.Disabled = false
	return u.ds.Put(ctx, user)
}

// updateUserLoginTime update user login time
func (u *userUsecaseImpl) updateUserLoginTime(ctx context.Context, user *model.User) error {
	user.LastLoginTime = time.Now().Time
	return u.ds.Put(ctx, user)
}

func convertUserModel(user *model.User) *apisv1.DetailUserResponse {
	return &apisv1.DetailUserResponse{
		UserBase: *convertUserBase(user),
		Projects: make([]apisv1.ProjectUserBase, 0),
	}
}

func convertUserBase(user *model.User) *apisv1.UserBase {
	return &apisv1.UserBase{
		Name:          user.Name,
		Alias:         user.Alias,
		Email:         user.Email,
		CreateTime:    user.CreateTime,
		LastLoginTime: user.LastLoginTime,
		Disabled:      user.Disabled,
	}
}

func GeneratePasswordHash(s string) (string, error) {
	if s == "" {
		return "", bcode.ErrUserInvalidPassword
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(s), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(hashed), nil
}

func compareHashWithPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
