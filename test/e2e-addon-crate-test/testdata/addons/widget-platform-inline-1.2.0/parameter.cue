// Addon parameters. With type: addon they come from the component's
// `properties.properties` map; the render also stores them in the Secret
// addon-secret-widget-platform-inline (only when the map is non-empty).
parameter: {
	// +usage=Greeting written into the addon's own ConfigMaps
	greeting: *"hello from widget-platform-inline" | string
	// +usage=Team recorded by the addon-level platform-owner-inline trait default
	team: *"platform" | string
}
