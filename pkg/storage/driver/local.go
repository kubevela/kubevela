package driver

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ghodss/yaml"

	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/appfile/template"
	"github.com/oam-dev/kubevela/pkg/utils/env"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

// LocalDriverName is local storage driver name
const LocalDriverName = "Local"

// Local Storage
type Local struct {
}

// NewLocalStorage get storage client of Local type
func NewLocalStorage() *Local {
	return &Local{}
}

// Name  is local storage driver name
func (l *Local) Name() string {
	return LocalDriverName
}

// List applications from local storage
func (l *Local) List(envName string) ([]*RespApplication, error) {
	appDir, err := getApplicationDir(envName)
	if err != nil {
		return nil, fmt.Errorf("get app dir from env %s err %w", envName, err)
	}
	files, err := ioutil.ReadDir(appDir)
	if err != nil {
		return nil, fmt.Errorf("list apps from %s err %w", appDir, err)
	}
	var apps []*RespApplication
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if !strings.HasSuffix(f.Name(), ".yaml") {
			continue
		}
		app, err := loadFromFile(filepath.Join(appDir, f.Name()))
		if err != nil {
			return nil, fmt.Errorf("load application err %w", err)
		}
		apps = append(apps, app)
	}
	return apps, nil
}

// Save application from local storage
func (l *Local) Save(app *RespApplication, envName string) error {
	appDir, err := getApplicationDir(envName)
	if err != nil {
		return fmt.Errorf("get app dir from env %s err %w", envName, err)
	}
	if app.CreateTime.IsZero() {
		app.CreateTime = time.Now()
	}
	app.UpdateTime = time.Now()
	out, err := yaml.Marshal(app)
	if err != nil {
		return err
	}
	//nolint:gosec
	return ioutil.WriteFile(filepath.Join(appDir, app.Name+".yaml"), out, 0644)
}

// Delete application from local storage
func (l *Local) Delete(envName, appName string) error {
	appDir, err := getApplicationDir(envName)
	if err != nil {
		return fmt.Errorf("get app dir from env %s err %w", envName, err)
	}
	return os.Remove(filepath.Join(appDir, appName+".yaml"))
}

// Get application from local storage
func (l *Local) Get(envName, appName string) (*RespApplication, error) {
	appDir, err := getApplicationDir(envName)
	if err != nil {
		return nil, fmt.Errorf("get app dir from env %s err %w", envName, err)
	}
	app, err := loadFromFile(filepath.Join(appDir, appName+".yaml"))

	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(`application "%s" not found`, appName)
		}
		return nil, err
	}

	return app, nil
}

func getApplicationDir(envName string) (string, error) {
	appDir := filepath.Join(env.GetEnvDirByName(envName), "applications")
	_, err := system.CreateIfNotExist(appDir)
	return appDir, err
}

// LoadFromFile will load application from file
func loadFromFile(fileName string) (*RespApplication, error) {
	tm, err := template.Load()
	if err != nil {
		return nil, err
	}
	_, err = os.Stat(fileName)
	if err != nil {
		return nil, err
	}

	f, err := appfile.LoadFromFile(fileName)
	if err != nil {
		return nil, err
	}
	app := &RespApplication{AppFile: f, Tm: tm}
	return app, nil
}
