/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gen_sdk

import "embed"

var (
	//go:embed openapi-generator/templates
	// Templates contains different template files for different languages
	Templates embed.FS
	// SupportedLangs is supported languages
	SupportedLangs = map[string]bool{"go": true}
	//go:embed _scaffold
	// Scaffold is scaffold files for different languages
	Scaffold embed.FS
	// ScaffoldDir is scaffold dir name
	ScaffoldDir = "_scaffold"
)
