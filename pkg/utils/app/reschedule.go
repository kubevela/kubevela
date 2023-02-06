/*
Copyright 2023 The KubeVela Authors.

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

package app

import (
	"context"
	"reflect"
	"strings"

	"github.com/kubevela/pkg/controller/sharding"
	"github.com/kubevela/pkg/util/slices"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
)

func reschedule(ctx context.Context, cli client.Client, o client.Object, shardID string) error {
	oldID, scheduled := sharding.GetScheduledShardID(o)
	if !scheduled || oldID != shardID {
		sharding.SetScheduledShardID(o, shardID)
		if err := cli.Update(ctx, o); err != nil {
			return err
		}
		klog.Infof("schedule %s/%s to %s", strings.ToLower(o.GetObjectKind().GroupVersionKind().Kind), o.GetName(), shardID)
	}
	return nil
}

// RescheduleAppRevAndRT reschedule ApplicationRevision and ResourceTracker of app to given shard
func RescheduleAppRevAndRT(ctx context.Context, cli client.Client, app *v1beta1.Application, shardID string) error {
	rt, currentRT, ts, crRT, err := resourcetracker.ListApplicationResourceTrackers(ctx, cli, app)
	if err != nil {
		return err
	}
	appRevs, err := application.GetAppRevisions(ctx, cli, app.Name, app.Namespace)
	if err != nil {
		return err
	}
	var objs []client.Object
	objs = append(objs, slices.Map(ts, func(r *v1beta1.ResourceTracker) client.Object { return r })...)
	objs = append(objs, []client.Object{crRT, currentRT, rt}...)
	objs = append(objs, slices.Map(appRevs, func(r v1beta1.ApplicationRevision) client.Object { return r.DeepCopy() })...)
	objs = append(objs, app)
	objs = slices.Filter(objs, func(o client.Object) bool {
		return o != nil && !reflect.ValueOf(o).IsNil()
	})
	errs := slices.ParMap(objs, func(o client.Object) error { return reschedule(ctx, cli, o, shardID) })
	return errors.NewAggregate(errs)
}
