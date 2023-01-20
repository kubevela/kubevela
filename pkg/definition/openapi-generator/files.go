package openapi_generator

import "embed"

var (
	//go:embed templates
	Tempaltes embed.FS
	//go:embed openapi-generator
	OpenapiGenerator []byte
	//
	SupportedLangs = map[string]bool{"go": true}
)
