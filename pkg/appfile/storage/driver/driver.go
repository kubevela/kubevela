package driver

import (
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
)

// Driver is mutli implement interface
type Driver interface {
	// List applications
	List(envName string) ([]*RespApplication, error)
	// Save application
	Save(app *RespApplication, envName string) error
	// Delete application
	Delete(envName, appName string) error
	// Get application
	Get(envName, appName string) (*RespApplication, error)
	// Name of storage driver
	Name() string
}

// RespApplication will get  application from Storage client
type RespApplication struct {
	*appfile.AppFile `json:",inline"`
	Tm               template.Manager `json:"-"`
}
