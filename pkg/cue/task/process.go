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

package task

import (
	"encoding/json"
	"errors"
	"fmt"

	"cuelang.org/go/cue"

	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/pkg/builtin"
	"github.com/oam-dev/kubevela/pkg/builtin/registry"
)

// Process processing the http task
func Process(val cue.Value) (cue.Value, error) {
	taskVal := val.LookupPath(value.FieldPath("processing", "http"))
	if !taskVal.Exists() {
		return val, errors.New("there is no http in processing")
	}
	resp, err := exec(taskVal)
	if err != nil {
		return val, fmt.Errorf("fail to exec http task, %w", err)
	}

	return val.FillPath(value.FieldPath("processing", "output"), resp), nil
}

func exec(v cue.Value) (map[string]interface{}, error) {
	got, err := builtin.RunTaskByKey("http", cue.Value{}, &registry.Meta{Obj: v})
	if err != nil {
		return nil, err
	}
	gotMap, ok := got.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("fail to convert got to map")
	}
	body, ok := gotMap["body"].(string)
	if !ok {
		return nil, fmt.Errorf("fail to convert body to string")
	}
	resp := make(map[string]interface{})
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, err
	}
	return resp, nil
}
