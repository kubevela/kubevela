package common

import (
	"reflect"

	v1 "k8s.io/api/core/v1"
)

// ServiceKind is string "Service"
var ServiceKind = reflect.TypeOf(v1.Service{}).Name()

// ServiceAPIVersion is string "v1"
var ServiceAPIVersion = v1.SchemeGroupVersion.String()
