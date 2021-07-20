package providers

import (
	"sync"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

// Handler is provider's processing method.
type Handler func(ctx wfContext.Context, v *value.Value, act types.Action) error

// Providers is provider discover interface.
type Providers interface {
	GetHandler(provider, name string) (Handler, bool)
	Register(provider string, m map[string]Handler)
}

type providers struct {
	l sync.RWMutex
	m map[string]map[string]Handler
}

// GetHandler get handler by provider name and handle name.
func (p *providers) GetHandler(providerName, handleName string) (Handler, bool) {
	p.l.RLock()
	defer p.l.RUnlock()
	provider, ok := p.m[providerName]
	if !ok {
		return nil, false
	}
	h, ok := provider[handleName]
	return h, ok
}

// Register install provider.
func (p *providers) Register(provider string, m map[string]Handler) {
	p.l.Lock()
	defer p.l.Unlock()
	p.m[provider] = m
}

// NewProviders will create provider discover.
func NewProviders() Providers {
	return &providers{m: map[string]map[string]Handler{}}
}
