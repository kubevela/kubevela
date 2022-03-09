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

package parallel

import (
	"reflect"
)

// ParInput input for parallel execution
type ParInput interface{}

// ParOutput output for parallel execution
type ParOutput interface{}

type orderedParOutput struct {
	ParOutput
	index int
}

// ParExecProto parallel execute handler function on parInputs, with maximum concurrency as parallelism
func ParExecProto(handler func(ParInput) ParOutput, parInputs []ParInput, parallelism int) []ParOutput {
	outs := make(chan orderedParOutput)
	pool := make(chan struct{}, parallelism)
	for _idx, _input := range parInputs {
		go func(idx int, input ParInput) {
			pool <- struct{}{}
			output := handler(input)
			outs <- orderedParOutput{ParOutput: output, index: idx}
			<-pool
		}(_idx, _input)
	}
	outputs := make([]ParOutput, len(parInputs))
	for range parInputs {
		out := <-outs
		outputs[out.index] = out.ParOutput
	}
	return outputs
}

// ParExec execute handler on parInputs, with automatic type conversion and maximum concurrency as parallelism
// Examples:
// > out := ParExec(func(x int) int { return x*x }, []int{1,2,3,4,5}, 5)
// < out: []int{1,4,19,16,25}
// > out := ParExec(func(x int, y string) (string, bool) { return y, x%2==0 }, [][]interface{}{{1,"n"},{2,"y"}}, 2)
// < out: [][]interface{{"n",false},{"y",true}}
func ParExec(handler interface{}, parInputs interface{}, parallelism int) interface{} {
	parInputsVal := reflect.ValueOf(parInputs)
	elemLen := parInputsVal.Len()
	_parInputVal := reflect.MakeSlice(reflect.TypeOf([]ParInput{}), elemLen, elemLen)
	for i := 0; i < elemLen; i++ {
		v := parInputsVal.Index(i)
		if v.IsValid() {
			_parInputVal.Index(i).Set(v)
		}
	}

	handleFunc := reflect.ValueOf(handler)
	handleFuncTyp := reflect.TypeOf(handler)
	nParams, nReturns := handleFuncTyp.NumIn(), handleFuncTyp.NumOut()
	parOutputTyp := reflect.TypeOf([]ParOutput{}).Elem()
	_handler := reflect.MakeFunc(reflect.TypeOf(func(ParInput) ParOutput { return nil }), func(args []reflect.Value) (results []reflect.Value) {
		in := make([]reflect.Value, nParams)
		_inputVal := args[0].Elem()
		if nParams > 1 {
			for i := 0; i < nParams; i++ {
				in[i] = _inputVal.Index(i).Elem()
			}
		} else if nParams == 1 {
			in[0] = _inputVal
		}
		for i := 0; i < nParams; i++ {
			if !in[i].IsValid() {
				in[i] = reflect.New(handleFuncTyp.In(i)).Elem()
			}
		}
		out := handleFunc.Call(in)

		_outputVal := reflect.New(parOutputTyp).Elem()
		if nReturns > 1 {
			ret := reflect.MakeSlice(reflect.TypeOf([]interface{}{}), nReturns, nReturns)
			for i := 0; i < nReturns; i++ {
				if out[i].IsValid() {
					ret.Index(i).Set(out[i])
				}
			}
			_outputVal.Set(ret)
		} else if nReturns == 1 {
			if out[0].IsValid() {
				_outputVal.Set(out[0])
			}
		}
		return []reflect.Value{_outputVal}
	})
	outs := ParExecProto(_handler.Interface().(func(ParInput) ParOutput), _parInputVal.Interface().([]ParInput), parallelism)
	if nReturns == 0 {
		return nil
	}
	var outputs reflect.Value
	if nReturns == 1 {
		outputs = reflect.MakeSlice(reflect.SliceOf(handleFuncTyp.Out(0)), elemLen, elemLen)
	} else {
		outputs = reflect.MakeSlice(reflect.TypeOf([]interface{}{}), elemLen, elemLen)
	}
	for i := 0; i < elemLen; i++ {
		v := reflect.ValueOf(outs[i])
		if v.IsValid() {
			outputs.Index(i).Set(v)
		}
	}
	return outputs.Interface()
}
