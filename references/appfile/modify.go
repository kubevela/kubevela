package appfile

import (
	"errors"

	"github.com/oam-dev/kubevela/references/appfile/api"
)

// SetWorkload will set user workload for Appfile
func SetWorkload(app *api.Application, componentName, workloadType string, workloadData map[string]interface{}) error {
	if app == nil {
		return errors.New("app is nil pointer")
	}

	s, ok := app.Services[componentName]
	if !ok {
		s = api.Service{}
	}
	s["type"] = workloadType
	for k, v := range workloadData {
		s[k] = v
	}
	app.Services[componentName] = s
	return Validate(app)
}

// SetTrait will set user trait for Appfile
func SetTrait(app *api.Application, componentName, traitType string, traitData map[string]interface{}) error {
	if app == nil {
		return errors.New("app is nil pointer")
	}
	if traitData == nil {
		traitData = make(map[string]interface{})
	}

	s, ok := app.Services[componentName]
	if !ok {
		s = api.Service{}
	}

	t, ok := s[traitType]
	if !ok {
		t = make(map[string]interface{})
	}
	tm := t.(map[string]interface{})
	for k, v := range traitData {
		tm[k] = v
	}
	s[traitType] = t
	app.Services[componentName] = s
	return Validate(app)
}

// RemoveTrait will remove a trait from Appfile
func RemoveTrait(app *api.Application, componentName, traitType string) error {
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

// RemoveComponent will remove component from Appfile
func RemoveComponent(app *api.Application, componentName string) error {
	if app == nil {
		return errors.New("app is nil pointer")
	}

	delete(app.Services, componentName)
	return nil
}
