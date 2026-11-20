parameter: {
	// +usage=Namespace the config store is installed into
	namespace: *"config-store-system" | string
	// +usage=Value written into the shared ConfigMap, asserted by the parameter passthrough spec
	greeting: *"hello" | string
}
