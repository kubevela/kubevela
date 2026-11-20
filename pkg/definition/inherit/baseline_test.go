/*
Copyright 2026 The KubeVela Authors.

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

package inherit

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
)

// The definition rendered the way it is today, with no inheritance in the
// picture at all: one compile of the template with its parameters and context,
// and nothing of this package involved.
//
// Every other number is only meaningful against this one. A chain of one should
// sit on top of it, since `Render` short-circuits before any of the machinery.
func BenchmarkBaselinePlainTemplate(b *testing.B) {
	root, err := os.ReadFile("testdata/webservice.cue")
	if err != nil {
		b.Fatal(err)
	}
	src := join(string(root), benchParams, benchContext)

	var compiles int64
	compile := countingCompile(&compiles)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		atomic.StoreInt64(&compiles, 0)
		v, err := compile(context.Background(), src)
		if err != nil {
			b.Fatal(err)
		}
		if !v.Exists() {
			b.Fatal("no value")
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(atomic.LoadInt64(&compiles)), "compiles/op")
}
