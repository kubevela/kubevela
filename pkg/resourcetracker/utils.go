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
	"sort"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

type sortResourceTrackerByVersion struct {
	rts     []*v1beta1.ResourceTracker
	reverse bool
}

func (s sortResourceTrackerByVersion) Len() int { return len(s.rts) }

func (s sortResourceTrackerByVersion) Swap(i, j int) { s.rts[i], s.rts[j] = s.rts[j], s.rts[i] }

func (s sortResourceTrackerByVersion) Less(i, j int) bool {
	if s.reverse {
		return s.rts[i].Spec.ApplicationGeneration > s.rts[j].Spec.ApplicationGeneration
	}
	return s.rts[i].Spec.ApplicationGeneration < s.rts[j].Spec.ApplicationGeneration
}

// SortResourceTrackersByVersion sort resourceTrackers by version
func SortResourceTrackersByVersion(rts []*v1beta1.ResourceTracker, descending bool) []*v1beta1.ResourceTracker {
	s := sortResourceTrackerByVersion{rts: rts, reverse: descending}
	sort.Sort(s)
	return s.rts
}
