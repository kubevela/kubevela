package driver

import (
	"errors"
	"fmt"

	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
)

// Application is an implementation level object for Appfile, all vela commands will access AppFile from Appliction struct here.
type Application struct {
	*appfile.AppFile `json:",inline"`
	Tm               template.Manager
}

// NewApplication will create application object
func NewApplication(f *appfile.AppFile, tm template.Manager) *Application {
	if f == nil {
		f = appfile.NewAppFile()
	}
	return &Application{AppFile: f, Tm: tm}
}

// Validate will validate whether an Appfile is valid.
func (app *Application) Validate() error {
	if app.Name == "" {
		return errors.New("name is required")
	}
	if len(app.Services) == 0 {
		return errors.New("at least one service is required")
	}
	for name, svc := range app.Services {
		for traitName, traitData := range svc.GetApplicationConfig() {
			if app.Tm.IsTrait(traitName) {
				if _, ok := traitData.(map[string]interface{}); !ok {
					return fmt.Errorf("trait %s in '%s' must be map", traitName, name)
				}
			}
		}
	}
	return nil
}

// Driver is mutli implement interface
type Driver interface {
	// List applications
	List(envName string) ([]*Application, error)
	// Save application
	Save(app *Application, envName string) error
	// Delete application
	Delete(envName, appName string) error
	// Get application
	Get(envName, appName string) (*Application, error)
	// Name of storage driver
	Name() string
}
