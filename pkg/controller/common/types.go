package common

import (
	"reflect"

	v1 "k8s.io/api/core/v1"
)

var ServiceKind = reflect.TypeOf(v1.Service{}).Name()

var ServiceAPIVersion = v1.SchemeGroupVersion.String()
