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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestSortResourceTrackerByVersion(t *testing.T) {
	r := require.New(t)
	var rts1, rts2 []*v1beta1.ResourceTracker
	for _, i := range []int64{3, 5, 4, 2, 7, 1, 8, 6} {
		rt := &v1beta1.ResourceTracker{Spec: v1beta1.ResourceTrackerSpec{
			ApplicationGeneration: i,
		}}
		rts1 = append(rts1, rt)
		rts2 = append(rts2, rt)
	}
	SortResourceTrackersByVersion(rts1, false)
	for i, rt := range rts1 {
		r.Equal(int64(i+1), rt.Spec.ApplicationGeneration)
	}
	SortResourceTrackersByVersion(rts2, true)
	for i, rt := range rts2 {
		r.Equal(int64(8-i), rt.Spec.ApplicationGeneration)
	}
}
