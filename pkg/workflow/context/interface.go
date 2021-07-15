package context

import "github.com/oam-dev/kubevela/pkg/cue/model"

type Context interface {
	Clone() Context
	GetComponent(name string) (componentManifest,error)
	PatchComponent(name string, patchValue *model.Value)error
	GetVar(paths ...string)(*model.Value,error)
	SetVar(v *model.Value, paths ...string) error
	Step()(string,int)
	Commit()error
	MakeParameter(parameter map[string]interface{}) (*model.Value, error)
}
