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

package model

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// Application is the application resource object
type Application struct {
	name       string
	namespace  string
	phase      string
	createTime string
}

// ApplicationList is application resource list
type ApplicationList []Application

// ToTableBody generate body of table in application view
func (l ApplicationList) ToTableBody() [][]string {
	data := make([][]string, len(l))
	for index, app := range l {
		data[index] = []string{app.name, app.namespace, app.phase, app.createTime}
	}
	return data
}

// ListApplications list all apps in all namespaces
func ListApplications(ctx context.Context, c client.Reader) (ApplicationList, error) {
	apps := v1beta1.ApplicationList{}
	namespace := ctx.Value(&CtxKeyNamespace).(string)

	if err := c.List(ctx, &apps, client.InNamespace(namespace)); err != nil {
		return ApplicationList{}, err
	}
	appList := make(ApplicationList, len(apps.Items))
	for index, app := range apps.Items {
		appList[index] = Application{app.Name, app.Namespace, string(app.Status.Phase), app.CreationTimestamp.String()}
	}
	return appList, nil
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

// applicationNum return the num of application
func applicationNum(ctx context.Context, c client.Client) (int, error) {
	apps := v1beta1.ApplicationList{}
	if err := c.List(ctx, &apps); err != nil {
		return 0, err
	}
	return len(apps.Items), nil
}

// runningApplicationNum return the num of running application
func runningApplicationNum(ctx context.Context, c client.Client) (int, error) {
	num := 0
	apps := v1beta1.ApplicationList{}
	if err := c.List(ctx, &apps); err != nil {
		return num, err
	}
	for _, app := range apps.Items {
		if app.Status.Phase == common.ApplicationRunning {
			num++
		}
	}
	return num, nil
}
