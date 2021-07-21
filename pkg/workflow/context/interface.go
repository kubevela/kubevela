package context

import (
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
)

// Context is workflow context interface
type Context interface {
	GetComponent(name string) (*ComponentManifest, error)
	PatchComponent(name string, patchValue *value.Value) error
	GetVar(paths ...string) (*value.Value, error)
	SetVar(v *value.Value, paths ...string) error
	Commit() error
	MakeParameter(parameter map[string]interface{}) (*value.Value, error)
	StoreRef() *runtimev1alpha1.TypedReference
}
