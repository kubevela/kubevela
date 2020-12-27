package application

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/appfile/storage/driver"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"

	"github.com/oam-dev/kubevela/pkg/oam"

	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

// BuildRun will build OAM and deploy from Appfile
func BuildRun(ctx context.Context, app *driver.Application, client client.Client, env *types.EnvMeta, io cmdutil.IOStreams) error {
	components, appconfig, scopes, err := OAM(app, env, io, true)
	if err != nil {
		return err
	}
	return Run(ctx, client, appconfig, components, scopes)
}

// Run will deploy OAM objects.
func Run(ctx context.Context, client client.Client,
	ac *v1alpha2.ApplicationConfiguration, comps []*v1alpha2.Component, scopes []oam.Object) error {
	for _, comp := range comps {
		if err := CreateOrUpdateComponent(ctx, client, comp); err != nil {
			return err
		}
	}
	if err := CreateScopes(ctx, client, scopes); err != nil {
		return err
	}
	return CreateOrUpdateAppConfig(ctx, client, ac)
}

// CreateOrUpdateComponent will create if not exist and update if exists.
func CreateOrUpdateComponent(ctx context.Context, client client.Client, comp *v1alpha2.Component) error {
	var getc v1alpha2.Component
	key := ctypes.NamespacedName{Name: comp.Name, Namespace: comp.Namespace}
	if err := client.Get(ctx, key, &getc); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		return client.Create(ctx, comp)
	}
	comp.ResourceVersion = getc.ResourceVersion
	return client.Update(ctx, comp)
}

// CreateOrUpdateAppConfig will create if not exist and update if exists.
func CreateOrUpdateAppConfig(ctx context.Context, client client.Client, appConfig *v1alpha2.ApplicationConfiguration) error {
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
		return client.Create(ctx, appConfig)
	}
	appConfig.ResourceVersion = geta.ResourceVersion
	return client.Update(ctx, appConfig)
}

// CreateScopes will create all scopes
func CreateScopes(ctx context.Context, client client.Client, scopes []oam.Object) error {
	for _, obj := range scopes {
		key := ctypes.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
		err := client.Get(ctx, key, obj)
		if err == nil {
			return nil
		}
		if !apierrors.IsNotFound(err) {
			return err
		}
		if err = client.Create(ctx, obj); err != nil {
			return err
		}
	}
	return nil
}
