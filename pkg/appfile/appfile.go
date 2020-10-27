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

var (
	ErrImageNotDefined = errors.New("image not defined")
)

const DefaultAppfilePath = "./vela.yaml"

type AppFile struct {
	Name       string             `json:"name"`
	Version    string             `json:"version"`
	CreateTime time.Time          `json:"createTime,omitempty"`
	UpdateTime time.Time          `json:"updateTime,omitempty"`
	Services   map[string]Service `json:"services"`
	Secrets    map[string]string  `json:"secrets"`

	configGetter configGetter
}

func NewAppFile() *AppFile {
	return &AppFile{
		Services:     make(map[string]Service),
		Secrets:      make(map[string]string),
		configGetter: defaultConfigGetter{},
	}
}

func Load() (*AppFile, error) {
	return LoadFromFile(DefaultAppfilePath)
}

func LoadFromFile(filename string) (*AppFile, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	af := NewAppFile()
	err = yaml.Unmarshal(b, af)
	if err != nil {
		return nil, err
	}
	return af, nil
}

// BuildOAM renders Appfile into AppConfig, Components. It also builds images for services if defined.
func (app *AppFile) BuildOAM(ns string, io cmdutil.IOStreams, tm template.Manager) (
	[]*v1alpha2.Component, *v1alpha2.ApplicationConfiguration, error) {
	return app.buildOAM(ns, io, true, tm)
}

// RenderOAM renders Appfile into AppConfig, Components.
func (app *AppFile) RenderOAM(ns string, io cmdutil.IOStreams, tm template.Manager) (
	[]*v1alpha2.Component, *v1alpha2.ApplicationConfiguration, error) {
	return app.buildOAM(ns, io, false, tm)
}

func (app *AppFile) buildOAM(ns string, io cmdutil.IOStreams, buildImage bool, tm template.Manager) (
	[]*v1alpha2.Component, *v1alpha2.ApplicationConfiguration, error) {

	appConfig := &v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: ns,
		},
	}

	var comps []*v1alpha2.Component

	for sname, svc := range app.GetServices() {
		var image string
		v, ok := svc["image"]
		if ok {
			image = v.(string)
		}

		if b := svc.GetBuild(); b != nil {
			if image == "" {
				return nil, nil, ErrImageNotDefined
			}
			if buildImage {
				io.Infof("\nBuilding service (%s)...\n", sname)
				if err := b.BuildImage(io, image); err != nil {
					return nil, nil, err
				}
			}
		}

		io.Infof("\nRendering configs for service (%s)...\n", sname)
		acComp, comp, err := svc.RenderService(tm, sname, ns, app.configGetter)
		if err != nil {
			return nil, nil, err
		}
		appConfig.Spec.Components = append(appConfig.Spec.Components, *acComp)
		comps = append(comps, comp)
	}

	return comps, appConfig, nil
}
