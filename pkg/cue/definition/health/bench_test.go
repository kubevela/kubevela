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

package health

import (
	"fmt"
	"testing"
)

// benchContext is a rendered Deployment, which is what a health policy is
// evaluated against and what gets marshalled into every compile.
func benchContext() map[string]interface{} {
	containers := make([]interface{}, 0, 3)
	for i := 0; i < 3; i++ {
		containers = append(containers, map[string]interface{}{
			"name":  fmt.Sprintf("c%d", i),
			"image": "nginx:1.27",
			"ports": []interface{}{map[string]interface{}{"containerPort": 8080 + i}},
			"env": []interface{}{
				map[string]interface{}{"name": "A", "value": "1"},
				map[string]interface{}{"name": "B", "value": "2"},
			},
			"resources": map[string]interface{}{
				"limits":   map[string]interface{}{"cpu": "1", "memory": "1Gi"},
				"requests": map[string]interface{}{"cpu": "500m", "memory": "512Mi"},
			},
		})
	}
	return map[string]interface{}{
		"output": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":       "billing-api",
				"namespace":  "acme",
				"generation": 3,
				"labels": map[string]interface{}{
					"app.oam.dev/component": "billing-api",
					"app.oam.dev/name":      "acme-billing",
				},
			},
			"spec": map[string]interface{}{
				"replicas": 3,
				"template": map[string]interface{}{"spec": map[string]interface{}{"containers": containers}},
			},
			"status": map[string]interface{}{
				"readyReplicas":      3,
				"updatedReplicas":    3,
				"replicas":           3,
				"observedGeneration": 3,
			},
		},
	}
}

// A policy of the shape the shipped webservice carries.
const benchPolicy = `
ready: {
	updatedReplicas:    *0 | int
	readyReplicas:      *0 | int
	replicas:           *0 | int
	observedGeneration: *0 | int
} & {
	if context.output.status.updatedReplicas != _|_ {updatedReplicas: context.output.status.updatedReplicas}
	if context.output.status.readyReplicas != _|_ {readyReplicas: context.output.status.readyReplicas}
	if context.output.status.replicas != _|_ {replicas: context.output.status.replicas}
	if context.output.status.observedGeneration != _|_ {observedGeneration: context.output.status.observedGeneration}
}
isHealth: (context.output.spec.replicas == ready.readyReplicas) && (context.output.spec.replicas == ready.updatedReplicas)
`

func benchHealthDepth(b *testing.B, depth int, everyLevel bool) {
	snippets := make([]Snippets, 0, depth)
	for i := 0; i < depth; i++ {
		switch {
		case i == depth-1:
			snippets = append(snippets, Snippets{Health: benchPolicy})
		case everyLevel:
			snippets = append(snippets, Snippets{Health: "isHealth: context.output.spec.replicas > 0"})
		default:
			snippets = append(snippets, Snippets{})
		}
	}

	ctx := benchContext()

	// The context is a workload with every replica ready, so the chain is
	// healthy. Timing it without checking would go on measuring a composition
	// that had started returning false.
	if healthy, err := checkHealthChain(ctx, snippets, nil); err != nil || !healthy {
		b.Fatalf("expected a healthy verdict before timing it, got %v (err %v)", healthy, err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := checkHealthChain(ctx, snippets, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// Only the root declares a policy, which is what an abstracting child looks
// like: nothing to compose, one evaluation.
func BenchmarkHealthDepth1(b *testing.B)         { benchHealthDepth(b, 1, false) }
func BenchmarkHealthDepth3RootOnly(b *testing.B) { benchHealthDepth(b, 3, false) }
func BenchmarkHealthDepth5RootOnly(b *testing.B) { benchHealthDepth(b, 5, false) }

// Every level declares one, which is the worst case: one evaluation per level.
func BenchmarkHealthDepth3EveryLevel(b *testing.B) { benchHealthDepth(b, 3, true) }
func BenchmarkHealthDepth5EveryLevel(b *testing.B) { benchHealthDepth(b, 5, true) }
