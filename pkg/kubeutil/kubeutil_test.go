/*
Copyright 2026 The KubeVela Authors.

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

package kubeutil_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/oam-dev/kubevela/pkg/kubeutil"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// The helpers are copies; these tests pin them to the originals so the two cannot drift
// while both exist.

func object(labels, annos map[string]string) *unstructured.Unstructured {
	o := &unstructured.Unstructured{Object: map[string]interface{}{}}
	o.SetLabels(labels)
	o.SetAnnotations(annos)
	return o
}

func TestLabelAndAnnotationHelpersMatchTheOriginals(t *testing.T) {
	inputs := []map[string]string{nil, {}, {"a": "1"}, {"a": "1", "b": "2"}}
	for _, existing := range inputs {
		for _, change := range inputs {
			keys := make([]string, 0, len(change))
			for k := range change {
				keys = append(keys, k)
			}
			mine, theirs := object(existing, existing), object(existing, existing)
			kubeutil.AddLabels(mine, change)
			oamutil.AddLabels(theirs, change)
			kubeutil.AddAnnotations(mine, change)
			oamutil.AddAnnotations(theirs, change)
			require.Equal(t, theirs.GetLabels(), mine.GetLabels())
			require.Equal(t, theirs.GetAnnotations(), mine.GetAnnotations())

			mine, theirs = object(existing, existing), object(existing, existing)
			kubeutil.RemoveLabels(mine, keys)
			oamutil.RemoveLabels(theirs, keys)
			kubeutil.RemoveAnnotations(mine, keys)
			oamutil.RemoveAnnotations(theirs, keys)
			require.Equal(t, theirs.GetLabels(), mine.GetLabels())
			require.Equal(t, theirs.GetAnnotations(), mine.GetAnnotations())
		}
	}
}

func TestExtractRevisionNumMatchesTheOriginal(t *testing.T) {
	for _, name := range []string{"app-v3", "app-v12", "my-app-v1", "v1", "appv2", "myapp-a1", "app-v", "app-vx"} {
		gotN, gotErr := kubeutil.ExtractRevisionNum(name, "-")
		wantN, wantErr := oamutil.ExtractRevisionNum(name, "-")
		require.Equal(t, wantN, gotN, name)
		require.Equal(t, wantErr == nil, gotErr == nil, name)
		if wantErr != nil {
			require.Equal(t, wantErr.Error(), gotErr.Error(), name)
		}
	}
}

func TestEscapeResourceNameToLabelValueMatchesTheOriginal(t *testing.T) {
	for _, name := range []string{"plain", "a:b", "a:b:c", ""} {
		require.Equal(t, utils.EscapeResourceNameToLabelValue(name), kubeutil.EscapeResourceNameToLabelValue(name))
	}
}

func TestIsNotFoundOrClusterNotExistsMatchesTheOriginal(t *testing.T) {
	for _, err := range []error{
		kerrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, "x"),
		multicluster.ErrClusterNotExists,
		kerrors.NewBadRequest("no such cluster: prod"),
		kerrors.NewConflict(schema.GroupResource{Resource: "configmaps"}, "x", nil),
	} {
		require.Equal(t, multicluster.IsNotFoundOrClusterNotExists(err), kubeutil.IsNotFoundOrClusterNotExists(err), err.Error())
	}
}
