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

import "fmt"

// SensitiveTurnedOffMessage is the message of the ConfigTemplate CRD's transition rule on
// spec.sensitive (apis/config.oam.dev/v1alpha1, ConfigTemplateSpec). The webhook and the CLI
// refuse the same update in the same words, so the refusal reads alike whichever layer answers.
const SensitiveTurnedOffMessage = "sensitive cannot be changed from true to false"

// SensitiveTurnedOffError is what the CLI, before it writes, and the ConfigTemplate webhook, at
// admission, give for an update that turns a template's sensitive off (issue #7410 section 7).
func SensitiveTurnedOffError(namespace, name string) error {
	return fmt.Errorf("config template %s/%s: %s", namespace, name, SensitiveTurnedOffMessage)
}
