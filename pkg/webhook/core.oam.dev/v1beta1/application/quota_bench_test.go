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

package application

import (
	"context"
	"fmt"
	"testing"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velacache "github.com/oam-dev/kubevela/pkg/cache"
)

// The count runs once per component type on every Application admission, so its
// cost against a full namespace decides whether the check is affordable.
//
// It reads a client-go Indexer, not the fake client. The fake client JSON
// round-trips the whole namespace on each List before applying the field selector,
// which hides the index and inflates every number by roughly 50x. The Indexer is
// the store an informer keeps, read the way CacheReader reads it: ByIndex on a
// "<namespace>/<value>" key, no deep copy.
const (
	benchFieldIndex = "field:" + velacache.DefinitionUsageIndex
	benchNSIndex    = toolscache.NamespaceIndex
)

// indexerReader serves List from an Indexer the way the informer cache does.
type indexerReader struct {
	client.Reader
	indexer toolscache.Indexer
}

func (r *indexerReader) List(_ context.Context, out client.ObjectList, opts ...client.ListOption) error {
	listOpts := client.ListOptions{}
	listOpts.ApplyOptions(opts)

	var (
		objs []interface{}
		err  error
	)
	if listOpts.FieldSelector != nil {
		req := listOpts.FieldSelector.Requirements()[0]
		objs, err = r.indexer.ByIndex(benchFieldIndex, listOpts.Namespace+"/"+req.Value)
	} else {
		objs, err = r.indexer.ByIndex(benchNSIndex, listOpts.Namespace)
	}
	if err != nil {
		return err
	}
	// The caller sets UnsafeDisableDeepCopy, so these go out as they sit.
	items := make([]runtime.Object, 0, len(objs))
	for _, o := range objs {
		items = append(items, o.(runtime.Object))
	}
	return apimeta.SetList(out, items)
}

// Half the Applications use the counted type, which is the half the index skips.
func benchmarkIndexer(b *testing.B, apps int) *indexerReader {
	b.Helper()
	indexer := toolscache.NewIndexer(toolscache.MetaNamespaceKeyFunc, toolscache.Indexers{
		benchNSIndex: toolscache.MetaNamespaceIndexFunc,
		benchFieldIndex: func(obj interface{}) ([]string, error) {
			app := obj.(*v1beta1.Application)
			used := velacache.DefinitionUsageOf(app)
			keys := make([]string, 0, len(used))
			for _, u := range used {
				keys = append(keys, app.Namespace+"/"+u)
			}
			return keys, nil
		},
	})
	for i := 0; i < apps; i++ {
		t := "webservice"
		if i%2 == 1 {
			t = "worker"
		}
		if err := indexer.Add(&v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("app-%d", i), Namespace: "tenant-a"},
			Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{
				{Name: "a", Type: t}, {Name: "b", Type: t},
			}},
		}); err != nil {
			b.Fatal(err)
		}
	}
	return &indexerReader{indexer: indexer}
}

func benchmarkCount(b *testing.B, apps int, indexed bool) {
	reader := benchmarkIndexer(b, apps)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := countUsage(ctx, reader, indexed, "tenant-a", velacache.UsageComponent, "webservice", "app-0"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCountComponentsOfType(b *testing.B) {
	for _, apps := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("indexed/%dapps", apps), func(b *testing.B) { benchmarkCount(b, apps, true) })
		b.Run(fmt.Sprintf("unindexed/%dapps", apps), func(b *testing.B) { benchmarkCount(b, apps, false) })
	}
}
