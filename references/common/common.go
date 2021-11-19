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

package common

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/appfile"
	"github.com/oam-dev/kubevela/references/appfile/api"
)

// BuildRun will build application and deploy from Appfile
func BuildRun(ctx context.Context, app *api.Application, client client.Client, namespace string, io util.IOStreams) error {
	o, scopes, err := app.BuildOAMApplication(namespace, io, app.Tm, true)
	if err != nil {
		return err
	}

	return appfile.Run(ctx, client, o, scopes)
}
