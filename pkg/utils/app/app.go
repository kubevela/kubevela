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

package app

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/component"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// ErrNotMatchRevision -
var ErrNotMatchRevision = errors.Errorf("failed to find revision matching the application")

// ErrPublishVersionNotChange -
var ErrPublishVersionNotChange = errors.Errorf("the PublishVersion is not changed")

// ErrRevisionNotChange -
var ErrRevisionNotChange = errors.Errorf("the revision is not changed")

// RevisionContextKey if this key is exit in ctx, we should use it preferentially
var RevisionContextKey = "revision-context-key"

// RollbackApplicationWithRevision make the exist application rollback to specified revision.
// revisionCtx the context used to manage the application revision.
func RollbackApplicationWithRevision(ctx context.Context, cli client.Client, appName, appNamespace, revisionName, publishVersion string) (*v1beta1.ApplicationRevision, *v1beta1.Application, error) {
	revisionCtx, ok := ctx.Value(&RevisionContextKey).(context.Context)
	if !ok {
		revisionCtx = ctx
	}
	// check revision
	revs, err := application.GetSortedAppRevisions(revisionCtx, cli, appName, appNamespace)
	if err != nil {
		return nil, nil, err
	}
	var matchedRev *v1beta1.ApplicationRevision
	for _, rev := range revs {
		if rev.Name == revisionName {
			matchedRev = rev.DeepCopy()
		}
	}
	if matchedRev == nil {
		return nil, nil, ErrNotMatchRevision
	}

	app := &v1beta1.Application{}
	if err := cli.Get(ctx, k8stypes.NamespacedName{Name: appName, Namespace: appNamespace}, app); err != nil {
		return nil, nil, err
	}

	if appPV := oam.GetPublishVersion(app); appPV == publishVersion {
		return nil, nil, ErrPublishVersionNotChange
	}

	if app != nil && app.Status.LatestRevision != nil && app.Status.LatestRevision.Name == revisionName {
		return nil, nil, ErrRevisionNotChange
	}

	// freeze the application
	appKey := client.ObjectKeyFromObject(app)
	controllerRequirement, err := utils.FreezeApplication(ctx, cli, app, func() {
		app.Spec = matchedRev.Spec.Application.Spec
		oam.SetPublishVersion(app, publishVersion)
	})
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to freeze application %s before update", appKey)
	}

	defer func() {
		// unfreeze application
		if err = utils.UnfreezeApplication(ctx, cli, app, nil, controllerRequirement); err != nil {
			klog.Errorf("failed to unfreeze application %s after update:%s", appKey, err.Error())
		}
	}()

	// create new revision based on the matched revision
	revName, revisionNum := utils.GetAppNextRevision(app)
	matchedRev.Name = revName
	oam.SetPublishVersion(matchedRev, publishVersion)
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(matchedRev)
	if err != nil {
		return nil, nil, err
	}
	un := &unstructured.Unstructured{Object: obj}
	component.ClearRefObjectForDispatch(un)
	if err = cli.Create(revisionCtx, un); err != nil {
		return nil, nil, errors.Wrapf(err, "failed to update application %s to create new revision %s", appKey, revName)
	}

	// update application status to point to the new revision
	if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err = cli.Get(ctx, appKey, app); err != nil {
			return err
		}
		app.Status = apicommon.AppStatus{
			LatestRevision: &apicommon.Revision{Name: revName, Revision: revisionNum, RevisionHash: matchedRev.GetLabels()[oam.LabelAppRevisionHash]},
		}
		return cli.Status().Update(ctx, app)
	}); err != nil {
		if delErr := cli.Delete(revisionCtx, un); delErr != nil {
			klog.Warningf("failed to clear the new revision after failed to rollback application:%s:%s", appName, delErr.Error())
		}
		return nil, nil, errors.Wrapf(err, "failed to update application %s to use new revision %s", appKey, revName)
	}
	return matchedRev, app, nil
}
