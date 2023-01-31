package gen_sdk

import "embed"

var (
	//go:embed openapi-generator/templates
	// Templates contains different template files for different languages
	Templates embed.FS
	//go:embed openapi-generator/openapi-generator
	// OpenapiGenerator is openapi-generator invoke script
	OpenapiGenerator []byte
	// SupportedLangs is supported languages
	SupportedLangs = map[string]bool{"go": true}
	//go:embed _scaffold
	// Scaffold is scaffold files for different languages
	Scaffold embed.FS
	// ScaffoldDir is scaffold dir name
	ScaffoldDir = "_scaffold"
)
