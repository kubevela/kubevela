package common

import (
	"context"

	"github.com/oam-dev/kubevela/references/apiserver/apis"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RetrieveComponent will get component status
func RetrieveComponent(ctx context.Context, c client.Reader, applicationName, componentName,
	namespace string) (apis.ComponentMeta, error) {
	var componentMeta apis.ComponentMeta
	applicationMeta, err := RetrieveApplicationStatusByName(ctx, c, applicationName, namespace)
	if err != nil {
		return componentMeta, err
	}

	for _, com := range applicationMeta.Components {
		if com.Name != componentName {
			continue
		}
		return com, nil
	}
	return componentMeta, nil
}
