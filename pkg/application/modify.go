package application

import (
	"errors"

	"github.com/oam-dev/kubevela/pkg/appfile"
)

func (app *Application) SetWorkload(componentName, workloadType string, workloadData map[string]interface{}) error {
	if app == nil {
		return errors.New("app is nil pointer")
	}

	s, ok := app.Services[componentName]
	if !ok {
		s = appfile.Service{}
	}
	s["type"] = workloadType
	for k, v := range workloadData {
		s[k] = v
	}
	return app.Validate()
}

func (app *Application) SetTrait(componentName, traitType string, traitData map[string]interface{}) error {
	if app == nil {
		return errors.New("app is nil pointer")
	}
	if traitData == nil {
		traitData = make(map[string]interface{})
	}

	s, ok := app.Services[componentName]
	if !ok {
		s = appfile.Service{}
	}

	t, ok := s[traitType]
	if !ok {
		t = make(map[string]interface{})
	}
	tm := t.(map[string]interface{})
	for k, v := range traitData {
		tm[k] = v
	}
	return app.Validate()
}

func (app *Application) RemoveTrait(componentName, traitType string) error {
	if app == nil {
		return errors.New("app is nil pointer")
	}

	s, ok := app.Services[componentName]
	if !ok {
		return nil
	}
	delete(s, traitType)
	return nil
}

func (app *Application) RemoveComponent(componentName string) error {
	if app == nil {
		return errors.New("app is nil pointer")
	}

	delete(app.Services, componentName)
	return nil
}
