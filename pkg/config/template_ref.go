/*
Copyright 2026 The KubeVela Authors.

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

package config

// TemplateRefImmutableMessage is the message of the Config CRD's transition rule on
// spec.templateRef (apis/config.oam.dev/v1alpha1, ConfigSpec). The rule is what refuses the
// change on a cluster whose admission webhooks are off, where the Config webhook never runs.
const TemplateRefImmutableMessage = "spec.templateRef is immutable: delete and recreate the config to use a different template"
