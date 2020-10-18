package appfile

import (
	"errors"
	"io/ioutil"
	"time"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/ghodss/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/pkg/appfile/template"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

const DefaultAppfilePath = "./vela.yml"

type AppFile struct {
	Name       string             `json:"name"`
	Version    string             `json:"version"`
	CreateTime time.Time          `json:"createTime,omitempty"`
	UpdateTime time.Time          `json:"updateTime,omitempty"`
	Services   map[string]Service `json:"services"`
	Secrets    map[string]string  `json:"secrets"`
}

func Load() (*AppFile, error) {
	return LoadFromFile(DefaultAppfilePath)
}

func LoadFromFile(filename string) (*AppFile, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	af := &AppFile{}
	err = yaml.Unmarshal(b, af)
	if err != nil {
		return nil, err
	}
	return af, af.Validate()
}

func (app *AppFile) Validate() error {
	if app.Name == "" {
		return errors.New("name is required")
	}
	if len(app.Services) == 0 {
		return errors.New("at least one service component is required")
	}
	return nil
}

// BuildOAM renders Appfile into AppConfig, Components. It also builds images for services if defined.
func (app *AppFile) BuildOAM(ns string, io cmdutil.IOStreams) (
	[]*v1alpha2.Component, *v1alpha2.ApplicationConfiguration, error) {
	io.Info("Loading templates ...")
	tm, err := template.Load()
	if err != nil {
		return nil, nil, err
	}

	appConfig := &v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: ns,
		},
	}

	var comps []*v1alpha2.Component

	for sname, svc := range app.GetServices() {
		build := svc.GetBuild()

		io.Infof("\nBuilding service (%s)...\n", sname)
		if err := build.BuildImage(io); err != nil {
			return nil, nil, err
		}

		io.Infof("\nRendering component configs for service (%s)...\n", sname)
		acComp, comp, err := svc.RenderService(tm, app.Name, ns, build.Image)
		if err != nil {
			return nil, nil, err
		}
		appConfig.Spec.Components = append(appConfig.Spec.Components, *acComp)
		comps = append(comps, comp)
	}

	return comps, appConfig, nil

}
