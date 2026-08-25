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

package celexpr

import (
	"fmt"
	"testing"
)

// These exist to keep the caching honest. Before it, one expression cost
// ~86,000 ns/op, of which ~644 ns was the evaluation and the rest was rebuilding
// an environment and recompiling text that had not changed.
//
// If BenchmarkEvalTreeSingleExpression regresses by an order of magnitude,
// something has stopped being cached - most likely DynEnv handing back a fresh
// environment, which makes the identity check in compiledFor miss on every call.

func benchValues() map[string]map[string]interface{} {
	return map[string]map[string]interface{}{
		"cfg": {"host": "example.com", "port": float64(8080), "tier": "prod"},
	}
}

func BenchmarkEvalTreeSingleExpression(b *testing.B) {
	vals, tree := benchValues(), map[string]interface{}{"v": "$(source.cfg.host)"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := EvalTree(tree, vals, map[string]interface{}{}); err != nil {
			b.Fatal(err)
		}
	}
}

// A properties blob closer to a real component: several expressions, nesting,
// interpolation and a list.
func BenchmarkEvalTreeRealisticProperties(b *testing.B) {
	vals := benchValues()
	tree := map[string]interface{}{
		"image":    "registry.example.com/app:$(source.cfg.tier)",
		"port":     "$(source.cfg.port)",
		"replicas": "$(source.cfg.tier == \"prod\" ? 6 : 1)",
		"env": []interface{}{
			map[string]interface{}{"name": "HOST", "value": "$(source.cfg.host)"},
			map[string]interface{}{"name": "ADDR", "value": "$(source.cfg.host):$(source.cfg.port)"},
		},
		"plain": "no expression here",
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := EvalTree(tree, vals, map[string]interface{}{}); err != nil {
			b.Fatal(err)
		}
	}
}

// The render path takes references for dependency ordering as well as
// evaluating, so both compile paths matter.
func BenchmarkReferences(b *testing.B) {
	env, err := DynEnv()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := References(env, "source.cfg.host"); err != nil {
			b.Fatal(err)
		}
	}
}

// Concurrent readers of one shared cache, which is the shape the render path
// takes once independent sources resolve in parallel.
func BenchmarkEvalParallel(b *testing.B) {
	env, err := DynEnv()
	if err != nil {
		b.Fatal(err)
	}
	in := map[string]interface{}{
		"source":  map[string]interface{}{"cfg": map[string]interface{}{"host": "a"}},
		"context": map[string]interface{}{},
	}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Eval(env, "source.cfg.host", in); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// A cache miss every time, so the cost of compiling is still visible and a
// regression in the miss path does not hide behind the hit path.
func BenchmarkEvalDistinctExpressions(b *testing.B) {
	env, err := DynEnv()
	if err != nil {
		b.Fatal(err)
	}
	in := map[string]interface{}{
		"source":  map[string]interface{}{"cfg": map[string]interface{}{"port": float64(1)}},
		"context": map[string]interface{}{},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Eval(env, fmt.Sprintf("source.cfg.port + %d", i), in); err != nil {
			b.Fatal(err)
		}
	}
}
