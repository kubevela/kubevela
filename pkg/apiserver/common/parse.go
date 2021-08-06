package common

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"

	"github.com/oam-dev/kubevela/pkg/apiserver/proto/model"
)

// ParseReference is used to include the common function `parseParameter`
type ParseReference struct {
	Client client.Client
}

// NewParseReference new parse reference
func NewParseReference(cli client.Client) *ParseReference {
	return &ParseReference{Client: cli}
}

// ParseDefinition parse definition
func (p *ParseReference) ParseDefinition(obj *unstructured.Unstructured, name, ns string) (*model.Definition, error) {
	var wd v1beta1.WorkloadDefinition
	err := kruntime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &wd)
	if err != nil {
		return nil, errors.Wrap(err, "fail to convert unstructured data to oam build-in WorkloadDefinition object")
	}

	if wd.Spec.Schematic == nil {
		return nil, errors.New("fail to get definition schematic")
	}

	capability := &types.Capability{
		Name:      name,
		Namespace: ns,
	}

	var jsonSchema string
	schematic := wd.Spec.Schematic
	if schematic.CUE != nil {
		capability.CueTemplate = schematic.CUE.Template
		jsonSchema, err = p.GenerateCUETemplateProperties(capability)
		if err != nil {
			return nil, err
		}
	}

	return &model.Definition{
		Name:       name,
		Namespace:  ns,
		Desc:       wd.GetAnnotations()[types.AnnDescription],
		Jsonschema: jsonSchema,
	}, nil
}
