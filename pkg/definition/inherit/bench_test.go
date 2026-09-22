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
	"fmt"
	"os"
	"sync/atomic"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// countingCompile is testCompile with a tally, so a benchmark can report how
// many times a chain compiles rather than only how long it took.
func countingCompile(n *int64) CompileFunc {
	return func(_ context.Context, src string) (cue.Value, error) {
		atomic.AddInt64(n, 1)
		v := cuecontext.New().CompileString(src)
		return v, v.Err()
	}
}

// middleLevel is a level that inherits its parent's schema and adds to it, which
// is the shape that makes a chain expensive: every level has to resolve the one
// above before its own parameters mean anything.
func middleLevel(i int) Level {
	return Level{
		Name: fmt.Sprintf("level-%d", i),
		Template: fmt.Sprintf(`
$super: properties: parameter

output: metadata: labels: "level-%d": parameter.tier%d

parameter: $super.parameter & {
	tier%d: *"standard" | string
}
`, i, i, i),
	}
}

func chainOfDepth(t testing.TB, depth int) []Level {
	t.Helper()
	root, err := os.ReadFile("testdata/webservice.cue")
	if err != nil {
		t.Fatal(err)
	}
	chain := make([]Level, 0, depth)
	for i := depth - 1; i >= 1; i-- {
		chain = append(chain, middleLevel(i))
	}
	return append(chain, Level{Name: "webservice", Template: string(root)})
}

const benchParams = `parameter: {image: "nginx:1.27", ports: [{port: 8080, expose: true}]}`

const benchContext = `
context: {
	name:      "billing-api"
	appName:   "acme-billing"
	namespace: "acme"
}
`

func benchDepth(b *testing.B, depth int) {
	chain := chainOfDepth(b, depth)
	var compiles int64

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		atomic.StoreInt64(&compiles, 0)
		if _, err := Render(context.Background(), chain, benchParams, benchContext,
			ComponentSurface, SameCompiler(countingCompile(&compiles))); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(atomic.LoadInt64(&compiles)), "compiles/op")
}

func BenchmarkRenderDepth1(b *testing.B) { benchDepth(b, 1) }
func BenchmarkRenderDepth2(b *testing.B) { benchDepth(b, 2) }
func BenchmarkRenderDepth3(b *testing.B) { benchDepth(b, 3) }
func BenchmarkRenderDepth4(b *testing.B) { benchDepth(b, 4) }
func BenchmarkRenderDepth5(b *testing.B) { benchDepth(b, 5) }

// The cap, so the worst chain anyone can write is measured rather than assumed.
func BenchmarkRenderDepth9(b *testing.B) { benchDepth(b, 9) }

// abstractingLevel declares its own parameters and reads nothing off `$super`,
// which is the shape most definitions will have: merging happens in Go, so a
// child that adds to its parent's output never mentions `$super.output`.
func abstractingLevel(i int) Level {
	return Level{
		Name: fmt.Sprintf("level-%d", i),
		Template: fmt.Sprintf(`
$super: properties: {image: parameter.image}

output: metadata: labels: "level-%d": parameter.tier%d

parameter: {
	image: string
	tier%d: *"standard" | string
}
`, i, i, i),
	}
}

func benchAbstracting(b *testing.B, depth int) {
	root, err := os.ReadFile("testdata/webservice.cue")
	if err != nil {
		b.Fatal(err)
	}
	chain := make([]Level, 0, depth)
	for i := depth - 1; i >= 1; i-- {
		chain = append(chain, abstractingLevel(i))
	}
	chain = append(chain, Level{Name: "webservice", Template: string(root)})

	var compiles int64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		atomic.StoreInt64(&compiles, 0)
		if _, err := Render(context.Background(), chain, benchParams, benchContext,
			ComponentSurface, SameCompiler(countingCompile(&compiles))); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(atomic.LoadInt64(&compiles)), "compiles/op")
}

func BenchmarkAbstractingDepth2(b *testing.B) { benchAbstracting(b, 2) }
func BenchmarkAbstractingDepth5(b *testing.B) { benchAbstracting(b, 5) }
func BenchmarkAbstractingDepth9(b *testing.B) { benchAbstracting(b, 9) }
