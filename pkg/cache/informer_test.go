package cache

import (
	"context"
	"testing"
	"time"

	"k8s.io/client-go/tools/cache"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type mockCache struct {
	ctrlcache.Cache
}

func (m *mockCache) GetInformer(ctx context.Context, obj client.Object, opts ...ctrlcache.InformerGetOption) (ctrlcache.Informer, error) {
	return &mockInformer{}, nil
}

type mockInformer struct {
	ctrlcache.Informer
}

func (m *mockInformer) AddEventHandler(handler cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	return nil, nil
}

func TestDefinitionCache_Start(t *testing.T) {
	in := NewDefinitionCache()
	ctx, cancel := context.WithCancel(context.Background())
	
	go in.Start(ctx, &mockCache{}, 10*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(10 * time.Millisecond)
}
