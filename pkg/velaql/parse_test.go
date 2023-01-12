/*
 Copyright 2021. The KubeVela Authors.

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

package velaql

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestParseVelaQL(t *testing.T) {
	testcases := []struct {
		ql    string
		query QueryView
		err   error
	}{{
		ql:  `view{test=,test1=hello}.output`,
		err: errors.New("key or value in parameter shouldn't be empty"),
	}, {
		ql:  `{test=1,app="name"}.Export`,
		err: errors.New("fail to parse the velaQL"),
	}, {
		ql:  `view.{test=true}.output.value.spec"`,
		err: errors.New("fail to parse the velaQL"),
	}, {
		ql: `view{test=1,app="name"}`,
		query: QueryView{
			View:   "view",
			Export: "status",
		},
		err: nil,
	}, {
		ql: `view{test=1,app="name"}.Export`,
		query: QueryView{
			View:   "view",
			Export: "Export",
		},
		err: nil,
	}, {
		ql: `view{test=true}.output.value.spec`,
		query: QueryView{
			View:   "view",
			Export: "output.value.spec",
		},
		err: nil,
	}, {
		ql: `view{test=true}.output.value[0].spec`,
		query: QueryView{
			View:   "view",
			Export: "output.value[0].spec",
		},
		err: nil,
	}}

	for _, testcase := range testcases {
		q, err := ParseVelaQL(testcase.ql)
		assert.Equal(t, testcase.err != nil, err != nil)
		if err == nil {
			assert.Equal(t, testcase.query.View, q.View)
			assert.Equal(t, testcase.query.Export, q.Export)
		} else {
			assert.Equal(t, testcase.err.Error(), err.Error())
		}
	}
}

func TestParseParameter(t *testing.T) {
	testcases := []struct {
		parameter    string
		parameterMap map[string]interface{}
		err          error
	}{{
		parameter: `{    }`,
		err:       errors.New("parameter shouldn't be empty"),
	}, {
		parameter: `{}`,
		err:       errors.New("parameter shouldn't be empty"),
	}, {
		parameter: `{ testString = "pod" , testFloat= , testBoolean=true}`,
		err:       errors.New("key or value in parameter shouldn't be empty"),
	}, {
		parameter: `{testString="pod",testFloat=1000.10,testBoolean=true,testInt=1}`,
		parameterMap: map[string]interface{}{
			"testString":  "pod",
			"testFloat":   1000.1,
			"testBoolean": true,
			"testInt":     int64(1),
		},
		err: nil,
	}, {
		parameter: `{testString="pod",testFloat=1000.10,testBoolean=true,testInt=1,}`,
		parameterMap: map[string]interface{}{
			"testString":  "pod",
			"testFloat":   1000.1,
			"testBoolean": true,
			"testInt":     int64(1),
		},
		err: nil,
	}, {
		parameter: `{ testString = "pod" , testFloat=1000.10 , testBoolean=true , testInt=1, }`,
		parameterMap: map[string]interface{}{
			"testString":  "pod",
			"testFloat":   1000.1,
			"testBoolean": true,
			"testInt":     int64(1),
		},
		err: nil,
	}}

	for _, testcase := range testcases {
		result, err := ParseParameter(testcase.parameter)
		assert.Equal(t, testcase.err != nil, err != nil)
		if err == nil {
			for k, v := range result {
				assert.Equal(t, testcase.parameterMap[k], v)
			}
		} else {
			assert.Equal(t, testcase.err.Error(), err.Error())
		}
	}
}
