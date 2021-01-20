package common

import "k8s.io/klog/v2"

const (
	// LogInfo level is for most info logs, this is the default
	// One should just call Info directly.
	LogInfo klog.Level = iota

	// LogDebug is for more verbose logs
	LogDebug

	// LogDebugWithContent is recommended if one wants to log with the content of the object,
	// ie. http body, json/yaml file content
	LogDebugWithContent

	// LogTrace is the most verbose log level, don't add anything after this
	LogTrace = 100
)
