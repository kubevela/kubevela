package storage

import (
	"github.com/oam-dev/kubevela/pkg/appfile/storage/driver"
)

// Store application store client
var Store *Storage

func init() {
	// TODO support provide multiple ways:
	// system environment
	// system configfile
	// startup arguments
	Store = NewStorage(driver.LocalDriverName)
}

// Storage is common storage client，use it to get app and others resource
type Storage struct {
	driver.Driver
}

// NewStorage form driver type
func NewStorage(driverName string) *Storage {
	// TODO remove driverName param ,should use environment get it
	// FIXME use env to get user storageDriver
	switch driverName {
	// TODO mutli implement Storage
	case driver.ConfigMapDriverName:
		return &Storage{driver.NewConfigMapStorage()}
	case driver.LocalDriverName:
		return &Storage{driver.NewLocalStorage()}
	default:
		return &Storage{driver.NewLocalStorage()}
	}
}

// List applications storage common implement
func (s *Storage) List(envName string) ([]*driver.Application, error) {
	return s.Driver.List(envName)
}

// Save application storage common implement
func (s *Storage) Save(app *driver.Application, envName string) error {
	return s.Driver.Save(app, envName)
}

// Delete application storage common implement
func (s *Storage) Delete(envName, appName string) error {
	return s.Driver.Delete(envName, appName)
}

// Get application storage common implement
func (s *Storage) Get(envName, appName string) (*driver.Application, error) {
	return s.Driver.Get(envName, appName)
}
