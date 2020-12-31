package task

import (
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/errors"
)

// Process processing the http task
func Process(inst *cue.Instance) (*cue.Instance, error) {
	processing, err := inst.Value().FieldByName("processing", true)
	if err != nil {
		return nil, err
	}
	if err := processing.Value.Validate(cue.Concrete(true), cue.Final()); err != nil {
		errList := errors.Errors(err)
		for _, e := range errList {
			fmt.Println(e.Error())
		}
		return nil, err
	}

	taskVal, err := processing.Value.FieldByName("http", true)
	if err != nil {
		return nil, fmt.Errorf("fail to fetch task from processing, %w", err)
	}

	resp, err := exec(taskVal.Value)
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
	htask := Lookup("http")
	runner, err := htask(cue.Value{})
	if err != nil {
		return nil, err
	}
	got, err := runner.Run(&Context{Obj: v})
	if err != nil {
		return nil, err
	}
	body := (got.(map[string]interface{}))["body"].(string)
	resp := make(map[string]interface{})
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, err
	}
	return resp, nil
}
