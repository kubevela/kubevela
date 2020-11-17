package types

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

// Args is args for controller-runtime client
type Args struct {
	Config *rest.Config
	Schema *runtime.Scheme
}
