/*
Copyright 2021 The KubeVela Authors.

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

package resourcetracker

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// GetApplicationRootResourceTracker get root resourcetracker for application
// root resourcetracker if a life-long resourcetracker which shares the life-cycle with the application instead of one revision
func GetApplicationRootResourceTracker(ctx context.Context, c client.Client, app *v1beta1.Application) (*v1beta1.ResourceTracker, error) {
	rt := &v1beta1.ResourceTracker{}
	rtName := app.Name + "-" + app.Namespace
	if err := c.Get(ctx, types.NamespacedName{Name: rtName}, rt); err != nil {
		return nil, err
	}
	return rt, nil
}

// CreateOrGetApplicationRootResourceTracker create or get root resourcetracker for application
// root resourcetracker if a life-long resourcetracker which shares the life-cycle with the application instead of one revision
func CreateOrGetApplicationRootResourceTracker(ctx context.Context, c client.Client, app *v1beta1.Application) (*v1beta1.ResourceTracker, error) {
	rt, err := GetApplicationRootResourceTracker(ctx, c, app)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
		rt = &v1beta1.ResourceTracker{}
		rtName := app.Name + "-" + app.Namespace
		rt.SetName(rtName)
		rt.SetLabels(map[string]string{
			oam.LabelAppName:      app.Name,
			oam.LabelAppNamespace: app.Namespace,
		})
		rt.SetLifeLong()
		if err = c.Create(ctx, rt); err != nil {
			return nil, err
		}
	}
	return rt, nil
}
