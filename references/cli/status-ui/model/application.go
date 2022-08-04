package model

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

type Application struct {
	name       string
	namespace  string
	phase      string
	createTime string
}

type ApplicationList struct {
	title []string
	data  []Application
}

func (l *ApplicationList) Header() []string {
	return l.title
}

func (l *ApplicationList) Body() [][]string {
	data := make([][]string, 0)
	for _, app := range l.data {
		data = append(data, []string{app.name, app.namespace, app.phase, app.createTime})
	}
	return data
}

// ListApplications list all apps in all namespaces
func ListApplications(ctx context.Context, c client.Reader) (*ApplicationList, error) {
	list := &ApplicationList{title: []string{"Name", "Namespace", "Phase", "CreateTime"}}
	apps := v1beta1.ApplicationList{}

	// all namespaces
	namespace := ""
	if err := c.List(ctx, &apps, client.InNamespace(namespace)); err != nil {
		if apierrors.IsNotFound(err) {
			return list, nil
		}
		return list, err
	}
	for _, app := range apps.Items {
		list.data = append(list.data, Application{app.Name, app.Namespace, string(app.Status.Phase), app.CreationTimestamp.String()})
	}

	return list, nil
}

// LoadApplication load the corresponding application according to name and namespace
func LoadApplication(c client.Client, name, ns string) (*v1beta1.Application, error) {
	app := new(v1beta1.Application)
	err := c.Get(context.Background(), client.ObjectKey{
		Name:      name,
		Namespace: ns,
	}, app)
	if err != nil {
		return nil, err
	}
	return app, nil
}
