package appfile

import (
	"os"

	"github.com/oam-dev/kubevela/pkg/utils/system"
	"github.com/oam-dev/kubevela/references/appfile/api"
	"github.com/oam-dev/kubevela/references/appfile/driver"
)

// Store application store client
var store *Storage

// Storage is common storage client，use it to get app and others resource
type Storage struct {
	api.Driver
}

// GetStorage will create storage driver from the system environment of "STORAGE_DRIVER"
func GetStorage() *Storage {
	driverName := os.Getenv(system.StorageDriverEnv)
	if store == nil || store.Name() != driverName {
		switch driverName {
		// TODO mutli implement Storage
		case driver.ConfigMapDriverName:
			store = &Storage{driver.NewConfigMapStorage()}
		case driver.LocalDriverName:
			store = &Storage{driver.NewLocalStorage()}
		default:
			store = &Storage{driver.NewLocalStorage()}
		}
	}
	return store
}

// Save application storage common implement
func (s *Storage) Save(app *api.Application, envName string) error {
	return s.Driver.Save(app, envName)
}

// Delete application storage common implement
func (s *Storage) Delete(envName, appName string) error {
	return s.Driver.Delete(envName, appName)
}
