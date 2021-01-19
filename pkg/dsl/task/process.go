package task

import (
	"encoding/json"
	"errors"
	"fmt"

	"cuelang.org/go/cue"

	"github.com/oam-dev/kubevela/pkg/builtin"
	"github.com/oam-dev/kubevela/pkg/builtin/registry"
)

// Process processing the http task
func Process(inst *cue.Instance) (*cue.Instance, error) {
	taskVal := inst.Lookup("processing", "http")
	if !taskVal.Exists() {
		return inst, errors.New("there is no http in processing")
	}
	resp, err := exec(taskVal)
	if err != nil {
		return nil, fmt.Errorf("fail to exec http task, %w", err)
	}

	appInst, err := inst.Fill(resp, "processing", "output")
	if err != nil {
		return nil, fmt.Errorf("fail to fill output from http, %w", err)
	}
	return appInst, nil
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
