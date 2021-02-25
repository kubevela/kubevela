package common

import (
	"reflect"

	v1 "k8s.io/api/core/v1"
)

const (
	// AutoscaleControllerName is the controller name of Trait autoscale
	AutoscaleControllerName = "autoscale"
	// MetricsControllerName is the controller name of Trait metrics
	MetricsControllerName = "metrics"
	// PodspecWorkloadControllerName is the controller name of Workload podsepcworkload
	PodspecWorkloadControllerName = "podspecworkload"
	// RouteControllerName is the controller name of Trait route
	RouteControllerName = "route"
	// RollingComponentsSep is the separator that divide the names in the newComponent annotation
	RollingComponentsSep = ","
	// DisableAllCaps disable all capabilities
	DisableAllCaps = "all"
	// DisableNoneCaps disable none of capabilities
	DisableNoneCaps = ""
)

// ServiceKind is string "Service"
var ServiceKind = reflect.TypeOf(v1.Service{}).Name()

// ServiceAPIVersion is string "v1"
var ServiceAPIVersion = v1.SchemeGroupVersion.String()
