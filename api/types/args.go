package types

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

type Args struct {
	Config *rest.Config
	Schema *runtime.Scheme
}
