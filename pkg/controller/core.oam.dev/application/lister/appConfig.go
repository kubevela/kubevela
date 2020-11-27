package lister

import (
	core "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// ApplicationConfigurationLister helps list ApplicationConfiguration.
type ApplicationConfigurationLister interface {
	// List lists all Appconfigs in the indexer.
	List(selector labels.Selector) (ret []*core.ApplicationConfiguration, err error)
	// Appconfigs returns an object that can list and get Appconfigs.
	AppConfigs(namespace string) AppConfigNamespaceLister
}

// AppconfigLister implements the AppconfigLister interface.
type appConfigLister struct {
	indexer cache.Indexer
}

// NewApplicationConfigurationLister returns a new AppconfigLister.
func NewApplicationConfigurationLister(indexer cache.Indexer) ApplicationConfigurationLister {
	return &appConfigLister{indexer: indexer}
}

// List lists all Appconfigs in the indexer.
func (s *appConfigLister) List(selector labels.Selector) (ret []*core.ApplicationConfiguration, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*core.ApplicationConfiguration))
	})
	return ret, err
}

// Appconfigs returns an object that can list and get Appconfigs.
func (s *appConfigLister) AppConfigs(namespace string) AppConfigNamespaceLister {
	return appConfigNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// AppConfigNamespaceLister helps list and get Appconfigs.
type AppConfigNamespaceLister interface {
	// List lists all Appconfigs in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*core.ApplicationConfiguration, err error)
	// Get retrieves the Appconfig from the indexer for a given namespace and name.
	Get(name string) (*core.ApplicationConfiguration, error)
}

// appConfigNamespaceLister implements the AppConfigNamespaceLister
// interface.
type appConfigNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all AppConfigs in the indexer for a given namespace.
func (s appConfigNamespaceLister) List(selector labels.Selector) (ret []*core.ApplicationConfiguration, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*core.ApplicationConfiguration))
	})
	return ret, err
}

// Get retrieves the Appconfig from the indexer for a given namespace and name.
func (s appConfigNamespaceLister) Get(name string) (*core.ApplicationConfiguration, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(core.SchemeGroupVersion.WithResource("appConfig").GroupResource(), name)
	}
	return obj.(*core.ApplicationConfiguration), nil
}
