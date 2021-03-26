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

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetType(t *testing.T) {
	svc1 := Service{}
	got := svc1.GetType()
	assert.Equal(t, DefaultWorkloadType, got)

	var workload2 = "W2"
	map2 := map[string]interface{}{
		"type": workload2,
		"cpu":  "0.5",
	}
	svc2 := Service(map2)
	got = svc2.GetType()
	assert.Equal(t, workload2, got)
}
