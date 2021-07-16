package context

import (
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/cue/model"
)

type Context interface {
	GetComponent(name string) (*componentManifest,error)
	PatchComponent(name string, patchValue *model.Value)error
	GetVar(paths ...string)(*model.Value,error)
	SetVar(v *model.Value, paths ...string) error
	Commit()error
	MakeParameter(parameter map[string]interface{}) (*model.Value, error)
	StoreRef() *runtimev1alpha1.TypedReference
}
