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
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/pointer"
)

func TestParExecNto1(t *testing.T) {
	var inputs [][]interface{}
	size := 100
	parallelism := 20
	for i := 0; i < size; i++ {
		if i%2 == 0 {
			inputs = append(inputs, []interface{}{i, i + 1, math.Sqrt(float64(i)), nil})
		} else {
			inputs = append(inputs, []interface{}{i, i + 1, math.Sqrt(float64(i)), pointer.Int(i)})
		}
	}
	outs := ParExec(func(a int, b int, c float64, d *int) *float64 {
		time.Sleep(time.Duration(rand.Intn(200)+25) * time.Millisecond)
		if d == nil {
			return nil
		}
		return pointer.Float64(float64(a*b) + c + float64(*d))
	}, inputs, parallelism)
	outputs, ok := outs.([]*float64)
	r := require.New(t)
	r.True(ok)
	r.Equal(size, len(outputs))
	for i, j := range outputs {
		if i%2 == 0 {
			r.Nil(j)
		} else {
			r.NotNil(j)
			r.Equal(float64(i*(i+1))+math.Sqrt(float64(i))+float64(i), *j)
		}
	}
}

func TestParExec0toN(t *testing.T) {
	var inputs [][]interface{}
	size := 100
	parallelism := 20
	for i := 0; i < size; i++ {
		inputs = append(inputs, nil)
	}
	outs := ParExec(func() (bool, string) {
		time.Sleep(time.Duration(rand.Intn(200)+25) * time.Millisecond)
		return false, "ok"
	}, inputs, parallelism)
	outputs, ok := outs.([]interface{})
	r := require.New(t)
	r.True(ok)
	r.Equal(size, len(outputs))
	for _, _j := range outputs {
		j, ok := _j.([]interface{})
		r.True(ok)
		r.Equal(2, len(j))
		j0, ok := j[0].(bool)
		r.True(ok)
		r.False(j0)
		j1, ok := j[1].(string)
		r.True(ok)
		r.Equal("ok", j1)
	}
}

func TestParExec1to0(t *testing.T) {
	var inputs []struct{ key int }
	size := 100
	parallelism := 20
	for i := 0; i < size; i++ {
		inputs = append(inputs, struct{ key int }{key: i})
	}
	outs := ParExec(func(obj struct{ key int }) {
		time.Sleep(time.Duration(rand.Intn(50)+size-obj.key) * time.Millisecond)
	}, inputs, parallelism)
	r := require.New(t)
	r.Nil(outs)
}
