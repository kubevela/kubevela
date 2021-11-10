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

package context

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
)

func TestLog(t *testing.T) {
	ctx := NewTraceContext(context.Background(), types.NamespacedName{
		Namespace: "default",
		Name:      "test-app",
	}.String())

	ctx.AddTag("controller", "application")
	ctx.Info("init")
	ctx.InfoDepth(1, "init")
	defer ctx.Commit("close")
	spanCtx := ctx.Fork("child1", DurationMetric(func(v float64) {
		fmt.Println(v)
	}))
	time.Sleep(time.Millisecond * 30)
	err := errors.New("mock error")
	ctx.Error(err, "test case", "generated", "test_log")
	ctx.ErrorDepth(1, err, "test case", "generated", "test_log")
	spanCtx.Commit("finished")

}
