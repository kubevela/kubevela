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
	"errors"
	"net/url"
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

func TestJoinURL(t *testing.T) {

	testcase := []struct {
		baseURL     string
		subPath     string
		expectedUrl string
		err         error
	}{
		{
			baseURL:     "https://www.kubevela.com",
			subPath:     "index.yaml",
			expectedUrl: "https://www.kubevela.com/index.yaml",
			err:         nil,
		},
		{
			baseURL:     "http://www.kubevela.com",
			subPath:     "index.yaml",
			expectedUrl: "http://www.kubevela.com/index.yaml",
			err:         nil,
		},
		{
			baseURL:     "0x7f:",
			subPath:     "index.yaml",
			expectedUrl: "",
			err:         &url.Error{Op: "parse", URL: "0x7f:", Err: errors.New("first path segment in URL cannot contain colon")},
		},
	}

	for _, tc := range testcase {
		url, err := JoinURL(tc.baseURL, tc.subPath)
		assert.Equal(t, tc.expectedUrl, url)
		assert.Equal(t, tc.err, err)
	}

}

func TestToString(t *testing.T) {
	type Obj struct {
		k string
	}
	obj := &Obj{"foo"}
	boolPtr := true
	caseA := []struct {
		input  interface{}
		expect string
	}{
		{int(666), "666"},
		{int8(6), "6"},
		{int16(6), "6"},
		{int32(6), "6"},
		{int64(6), "6"},
		{uint(6), "6"},
		{uint8(6), "6"},
		{uint16(6), "6"},
		{uint32(6), "6"},
		{uint64(6), "6"},
		{float32(3.14), "3.14"},
		{float64(3.14), "3.14"},
		{true, "true"},
		{false, "false"},
		{&boolPtr, "true"},
		{nil, ""},
		{[]byte("one time"), "one time"},
		{"one more time", "one more time"},
		{obj, ""},
	}

	for _, test := range caseA {
		v := ToString(test.input)
		assert.Equal(t, test.expect, v)
	}
}
