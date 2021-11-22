/*Copyright 2021 The KubeVela Authors.

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

package recorder

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	apps "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

const (
	// LabelRecordSource is label that describe recorder source.
	LabelRecordSource = "vela.io/source"
	// LabelRecordVersion is label that describe recorder version.
	LabelRecordVersion = "vela.io/wf-revision"
)

type recorder struct {
	cli    client.Client
	source *v1beta1.Application
	err    error
}

// With a recorder store.
func With(cli client.Client, source *v1beta1.Application) Store {
	return &recorder{
		cli:    cli,
		source: source,
	}
}

// Save object to controllerRevision.
func (r *recorder) Save(version string, data []byte) Store {
	if r.err != nil {
		return r
	}
	rv := &apps.ControllerRevision{}
	rv.Namespace = r.source.GetNamespace()
	rv.Revision = time.Now().UnixNano()

	if version == "" {
		wfStatus := r.source.Status.Workflow
		if wfStatus != nil {
			if !strings.Contains(wfStatus.AppRevision, ":") {
				version = wfStatus.AppRevision
			}
		}
	}

	if version == "" {
		version = fmt.Sprint(rv.Revision)
	}

	rv.Name = fmt.Sprintf("record-%s-%s", r.source.Name, version)

	rv.SetLabels(map[string]string{
		LabelRecordSource:  r.source.GetName(),
		LabelRecordVersion: version,
	})
	ownerRef := metav1.NewControllerRef(r.source, r.source.GroupVersionKind())
	ownerRef.APIVersion = v1beta1.SchemeGroupVersion.String()
	ownerRef.Kind = v1beta1.ApplicationKind
	rv.SetOwnerReferences([]metav1.OwnerReference{*ownerRef})
	rv.Data = runtime.RawExtension{
		Raw: data,
	}
	if err := r.cli.Create(context.Background(), rv); err != nil {
		if kerrors.IsAlreadyExists(err) {
			// ControllerRevision implements an immutable snapshot of state data
			// Once a ControllerRevision has been successfully created, it can not be updated.
			// So we need to delete the old one and create a new one.
			r.err = r.cli.Delete(context.Background(), &apps.ControllerRevision{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rv.Name,
					Namespace: rv.Namespace,
				},
			})
			r.err = r.cli.Create(context.Background(), rv)
		} else {
			r.err = errors.WithMessagef(err, "save record %s/%s", rv.Namespace, rv.Name)
		}
	}
	return r
}

// Limit gc over limit controllerRevisions.
func (r *recorder) Limit(max int) Store {
	if r.err != nil {
		return r
	}
	selector, err := labels.Parse(fmt.Sprintf("%s=%s", LabelRecordSource, r.source.GetName()))
	if err != nil {
		r.err = errors.WithMessagef(err, "limit recorder: make selector(source=%s)", r.source.GetName())
		return r
	}
	rds := &apps.ControllerRevisionList{}
	if err := r.cli.List(context.Background(), rds, &client.ListOptions{
		LabelSelector: selector,
	}); err != nil {
		r.err = errors.WithMessagef(err, "limit recorder: list controllerRevision (source=%s)", r.source.GetName())
	}
	if len(rds.Items) <= max {
		return r
	}
	items := rds.Items
	sort.Sort(rvSorter(items))
	for i := 0; i < len(rds.Items)-max; i++ {
		o := &items[i]
		if err := r.cli.Delete(context.Background(), o); err != nil {
			r.err = errors.WithMessage(err, "limit recorder: delete controllerRevision")
			break
		}
	}
	return r
}

// Error return error info.
func (r *recorder) Error() error {
	return r.err
}

// Store is an object that record info.
type Store interface {
	Save(version string, data []byte) Store
	Limit(max int) Store
	Error() error
}

type rvSorter []apps.ControllerRevision

// Len is the number of elements in the collection.
func (rs rvSorter) Len() int {
	return len(rs)
}

// Less reports whether the element with index i must sort before the element with index j.
func (rs rvSorter) Less(i, j int) bool {
	return rs[i].Revision < rs[j].Revision
}

// Swap swaps the elements with indexes i and j.
func (rs rvSorter) Swap(i, j int) {
	rs[i], rs[j] = rs[j], rs[i]
}
