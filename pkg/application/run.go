package application

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile/storage/driver"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// BuildRun will build application and deploy from Appfile
func BuildRun(ctx context.Context, app *driver.Application, client client.Client, env *types.EnvMeta, io cmdutil.IOStreams) error {
	o, scopes, err := app.BuildOAMApplication(env, io, app.Tm, true)
	if err != nil {
		return err
	}

	return Run(ctx, client, o, scopes)
}

// Run will deploy OAM objects and other assistant K8s Objects including ConfigMap, OAM Scope Custom Resource.
func Run(ctx context.Context, client client.Client, app *v1alpha2.Application, assistantObjects []oam.Object) error {
	if err := CreateOrUpdateObjects(ctx, client, assistantObjects); err != nil {
		return err
	}
	if app != nil {
		if err := CreateOrUpdateApplication(ctx, client, app); err != nil {
			return err
		}
	}
	return nil
}

// CreateOrUpdateObjects will create all scopes
func CreateOrUpdateObjects(ctx context.Context, client client.Client, objects []oam.Object) error {
	for _, obj := range objects {
		key := ctypes.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
		err := client.Get(ctx, key, u)
		if err == nil {
			obj.SetResourceVersion(u.GetResourceVersion())
			return client.Update(ctx, obj)
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

// CreateOrUpdateApplication will create if not exist and update if exists.
func CreateOrUpdateApplication(ctx context.Context, client client.Client, app *v1alpha2.Application) error {
	var geta v1alpha2.Application
	key := ctypes.NamespacedName{Name: app.Name, Namespace: app.Namespace}
	var exist = true
	if err := client.Get(ctx, key, &geta); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		exist = false
	}
	if !exist {
		return client.Create(ctx, app)
	}
	app.ResourceVersion = geta.ResourceVersion
	return client.Update(ctx, app)
}
