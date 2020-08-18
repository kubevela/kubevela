package application

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/cloud-native-application/rudrx/api/types"
	ctypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (app *Application) Run(ctx context.Context, client client.Client, env *types.EnvMeta) error {
	components, appconfig, err := app.OAM(env)
	if err != nil {
		return err
	}
	for _, cmp := range components {
		if err = CreateOrUpdateComponent(ctx, client, cmp); err != nil {
			return err
		}
	}
	return CreateOrUpdateAppConfig(ctx, client, appconfig)
}

func CreateOrUpdateComponent(ctx context.Context, client client.Client, comp v1alpha2.Component) error {
	var getc v1alpha2.Component
	key := ctypes.NamespacedName{Name: comp.Name, Namespace: comp.Namespace}
	var exist = true
	if err := client.Get(ctx, key, &getc); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		exist = false
	}
	if !exist {
		return client.Create(ctx, &comp)
	}
	comp.ResourceVersion = getc.ResourceVersion
	return client.Update(ctx, &comp)
}

func CreateOrUpdateAppConfig(ctx context.Context, client client.Client, appConfig v1alpha2.ApplicationConfiguration) error {
	var geta v1alpha2.ApplicationConfiguration
	key := ctypes.NamespacedName{Name: appConfig.Name, Namespace: appConfig.Namespace}
	var exist = true
	if err := client.Get(ctx, key, &geta); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		exist = false
	}
	if !exist {
		return client.Create(ctx, &appConfig)
	}
	appConfig.ResourceVersion = geta.ResourceVersion
	return client.Update(ctx, &appConfig)
}
