/*
 Copyright 2022. The KubeVela Authors.

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

package query

import (
	"encoding/json"

	cuejson "cuelang.org/go/pkg/encoding/json"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
)

// fillQueryResult help fill query result which contains k8s object to *value.Value
func fillQueryResult(v *value.Value, res interface{}, paths ...string) error {
	b, err := json.Marshal(res)
	if err != nil {
		return v.FillObject(err, "err")
	}
	expr, err := cuejson.Unmarshal(b)
	if err != nil {
		return v.FillObject(err, "err")
	}
	return v.FillObject(expr, paths...)
}
