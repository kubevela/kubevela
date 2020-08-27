package application

import (
	"errors"
	"strings"
)

func (app *Application) SetWorkload(componentName, workloadType string, workloadData map[string]interface{}) error {
	if app == nil {
		return errors.New("app is nil pointer")
	}
	if workloadData == nil {
		workloadData = make(map[string]interface{})
	}
	workloadData["name"] = strings.ToLower(componentName)
	if app.Components == nil {
		app.Components = make(map[string]map[string]interface{})
	}
	app.Components[componentName] = map[string]interface{}{
		workloadType: workloadData,
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
	traitData["name"] = strings.ToLower(traitType)
	if app.Components == nil {
		app.Components = make(map[string]map[string]interface{})
	}
	comp := app.Components[componentName]
	if comp == nil {
		comp = make(map[string]interface{})
	}
	traits, err := app.GetTraits(componentName)
	if err != nil {
		return err
	}
	traits[traitType] = traitData
	comp[Traits] = traits
	app.Components[componentName] = comp
	return app.Validate()
}

func (app *Application) RemoveTrait(componentName, traitType string) error {
	if app == nil {
		return errors.New("app is nil pointer")
	}
	if app.Components == nil {
		app.Components = make(map[string]map[string]interface{})
	}
	comp := app.Components[componentName]
	if comp == nil {
		comp = make(map[string]interface{})
	}
	traits, err := app.GetTraits(componentName)
	if err != nil {
		return err
	}
	delete(traits, traitType)
	comp[Traits] = traits
	app.Components[componentName] = comp
	return app.Validate()
}

func (app *Application) RemoveComponent(componentName string) error {
	if app == nil {
		return errors.New("app is nil pointer")
	}
	if app.Components == nil {
		app.Components = make(map[string]map[string]interface{})
	}
	delete(app.Components, componentName)
	return app.Validate()
}
