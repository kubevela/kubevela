package lister

import (
	core "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// ComponentLister helps list Component.
type ComponentLister interface {
	// List lists all Components in the indexer.
	List(selector labels.Selector) (ret []*core.Component, err error)
	// Components returns an object that can list and get Components.
	Components(namespace string) ComponentNamespaceLister
}

// componentLister implements the ComponentLister interface.
type componentLister struct {
	indexer cache.Indexer
}

// NewComponentLister returns a new ComponentLister.
func NewComponentLister(indexer cache.Indexer) ComponentLister {
	return &componentLister{indexer: indexer}
}

// List lists all Components in the indexer.
func (s *componentLister) List(selector labels.Selector) (ret []*core.Component, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*core.Component))
	})
	return ret, err
}

// Components returns an object that can list and get Components.
func (s *componentLister) Components(namespace string) ComponentNamespaceLister {
	return componentNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// ComponentNamespaceLister helps list and get Components.
type ComponentNamespaceLister interface {
	// List lists all Components in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*core.Component, err error)
	// Get retrieves the Component from the indexer for a given namespace and name.
	Get(name string) (*core.Component, error)
}

// ComponentNamespaceLister implements the ComponentNamespaceLister
// interface.
type componentNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all Components in the indexer for a given namespace.
func (s componentNamespaceLister) List(selector labels.Selector) (ret []*core.Component, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*core.Component))
	})
	return ret, err
}

// Get retrieves the Component from the indexer for a given namespace and name.
func (s componentNamespaceLister) Get(name string) (*core.Component, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(core.SchemeGroupVersion.WithResource("Component").GroupResource(), name)
	}
	return obj.(*core.Component), nil
}
