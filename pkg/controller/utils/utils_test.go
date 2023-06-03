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

package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestConstructExtract(t *testing.T) {
	tests := []string{"tam1", "test-comp", "xx", "tt-x-x-c"}
	revisionNum := []int{1, 5, 10, 100000}
	for idx, componentName := range tests {
		t.Run(fmt.Sprintf("tests %d for component[%s]", idx, componentName), func(t *testing.T) {
			revisionName := ConstructRevisionName(componentName, int64(revisionNum[idx]))
			got := ExtractComponentName(revisionName)
			if got != componentName {
				t.Errorf("want to get %s from %s but got %s", componentName, revisionName, got)
			}
			revision, _ := ExtractRevision(revisionName)
			if revision != revisionNum[idx] {
				t.Errorf("want to get %d from %s but got %d", revisionNum[idx], revisionName, revision)
			}
		})
	}
	badRevision := []string{"xx", "yy-", "zz-0.1"}
	t.Run(fmt.Sprintf("tests %s for extractRevision", badRevision), func(t *testing.T) {
		for _, revisionName := range badRevision {
			_, err := ExtractRevision(revisionName)
			if err == nil {
				t.Errorf("want to get err from %s but got nil", revisionName)
			}
		}
	})
}

func TestGetAppRevison(t *testing.T) {
	revisionName, latestRevision := GetAppNextRevision(nil)
	assert.Equal(t, revisionName, "")
	assert.Equal(t, latestRevision, int64(0))
	// the first is always 1
	app := &v1beta1.Application{}
	app.Name = "myapp"
	revisionName, latestRevision = GetAppNextRevision(app)
	assert.Equal(t, revisionName, "myapp-v1")
	assert.Equal(t, latestRevision, int64(1))
	app.Status.LatestRevision = &common.Revision{
		Name:     "myapp-v1",
		Revision: 1,
	}
	// we always automatically advance the revision
	revisionName, latestRevision = GetAppNextRevision(app)
	assert.Equal(t, revisionName, "myapp-v2")
	assert.Equal(t, latestRevision, int64(2))
}
