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

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareSlice(t *testing.T) {
	caseA := []string{"c", "b", "a"}
	caseB := []string{"c", "b", "a"}
	gotab, gotao, gotbo := ThreeWaySliceCompare(caseA, caseB)
	assert.Equal(t, gotab, []string{"a", "b", "c"})
	assert.Equal(t, gotao, []string(nil))
	assert.Equal(t, gotbo, []string(nil))

	caseA = []string{"c", "b"}
	caseB = []string{"c", "a"}
	gotab, gotao, gotbo = ThreeWaySliceCompare(caseA, caseB)
	assert.Equal(t, gotab, []string{"c"})
	assert.Equal(t, gotao, []string{"b"})
	assert.Equal(t, gotbo, []string{"a"})

	caseA = []string{"c", "b"}
	caseB = nil
	gotab, gotao, gotbo = ThreeWaySliceCompare(caseA, caseB)
	assert.Equal(t, gotab, []string(nil))
	assert.Equal(t, gotao, []string{"b", "c"})
	assert.Equal(t, gotbo, []string(nil))

	caseA = nil
	caseB = []string{"c", "b"}
	gotab, gotao, gotbo = ThreeWaySliceCompare(caseA, caseB)
	assert.Equal(t, gotab, []string(nil))
	assert.Equal(t, gotao, []string(nil))
	assert.Equal(t, gotbo, []string{"b", "c"})
}

func TestEqual(t *testing.T) {
	caseA := []string{"c", "b", "a"}
	caseB := []string{"c", "b", "a"}
	assert.Equal(t, EqualSlice(caseA, caseB), true)

	caseA = []string{"c", "a", "b"}
	caseB = []string{"c", "b", "a"}
	assert.Equal(t, EqualSlice(caseA, caseB), true)

	caseA = []string{"c", "a", "b"}
	caseB = []string{"b", "a", "c"}
	assert.Equal(t, EqualSlice(caseA, caseB), true)

	caseA = []string{"c", "a", "b"}
	caseB = []string{"b", "a", "c", "d"}
	assert.Equal(t, EqualSlice(caseA, caseB), false)

	caseA = []string{}
	caseB = []string{}
	assert.Equal(t, EqualSlice(caseA, caseB), true)

	caseA = []string{}
	caseB = []string{"b", "a", "c"}
	assert.Equal(t, EqualSlice(caseA, caseB), false)
}

func TestSliceIncludeSlice(t *testing.T) {
	caseA := []string{"b", "a", "c"}
	caseB := []string{}
	assert.Equal(t, SliceIncludeSlice(caseA, caseB), true)

	caseA = []string{"b", "a", "c"}
	caseB = []string{"b"}
	assert.Equal(t, SliceIncludeSlice(caseA, caseB), true)

	caseA = []string{"b", "a", "c"}
	caseB = []string{"b", "c"}
	assert.Equal(t, SliceIncludeSlice(caseA, caseB), true)

	caseA = []string{"b", "a", "c"}
	caseB = []string{"b", "c", "d"}
	assert.Equal(t, SliceIncludeSlice(caseA, caseB), false)

	caseA = []string{"b", "a", "c"}
	caseB = []string{"b", "c", "a"}
	assert.Equal(t, SliceIncludeSlice(caseA, caseB), true)
}
