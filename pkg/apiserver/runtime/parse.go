package runtime

import (
	"encoding/json"

	structpb "google.golang.org/protobuf/types/known/structpb"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	corecommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

	"github.com/oam-dev/kubevela/pkg/apiserver/proto/model"
)

const (
	// DefaultAppNamespace default namespace for application
	DefaultAppNamespace = "default"
)

// ParseCoreApplication parse app info
func ParseCoreApplication(obj *model.Application) (oamcore.Application, error) {
	var components []corecommon.ApplicationComponent
	app := NewApplication(obj.Name, obj.Namespace)
	for _, objComponent := range obj.GetComponents() {
		properties, err := objComponent.Properties.MarshalJSON()
		if err != nil {
			return app, err
		}

		var traits []corecommon.ApplicationTrait
		for _, objTrait := range objComponent.Traits {
			properties, err := objTrait.Properties.MarshalJSON()
			if err != nil {
				return app, err
			}
			trait := corecommon.ApplicationTrait{
				Type: objTrait.Type,
				Properties: runtime.RawExtension{
					Raw: properties,
				},
			}

			traits = append(traits, trait)
		}

		component := corecommon.ApplicationComponent{
			Name: objComponent.Name,
			Type: objComponent.Type,
			Properties: runtime.RawExtension{
				Raw: properties,
			},
			Traits: traits,
		}
		components = append(components, component)
	}
	app.Spec.Components = components

	return app, nil
}

// NewApplication create new application
func NewApplication(name, namespace string) oamcore.Application {
	if len(namespace) == 0 {
		namespace = DefaultAppNamespace
	}

	return oamcore.Application{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// ParseApplicationYaml parse app in yaml
func ParseApplicationYaml(obj *oamcore.Application) (*model.Application, error) {
	var components []*model.ComponentType
	for _, objComponent := range obj.Spec.Components {
		var comProperties structpb.Struct
		comProper, err := objComponent.Properties.MarshalJSON()
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(comProper, &comProperties)
		if err != nil {
			return nil, err
		}
		var traits []*model.TraitType
		for _, objTrait := range objComponent.Traits {
			var traProperties structpb.Struct
			traProper, err := objTrait.Properties.MarshalJSON()
			if err != nil {
				return nil, err
			}
			err = json.Unmarshal(traProper, &traProperties)
			if err != nil {
				return nil, err
			}
			trait := model.TraitType{
				Type:       objTrait.Type,
				Properties: &traProperties,
			}
			traits = append(traits, &trait)
		}
		comp := model.ComponentType{
			Name:       objComponent.Name,
			Type:       objComponent.Type,
			Namespace:  obj.Namespace,
			Properties: &comProperties,
			Traits:     traits,
		}
		components = append(components, &comp)
	}
	app := model.Application{
		Name:       obj.Name,
		Namespace:  obj.Namespace,
		UpdatedAt:  obj.CreationTimestamp.Unix(),
		Components: components,
	}
	return &app, nil
}
