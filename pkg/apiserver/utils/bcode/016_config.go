/*
Copyright 2022 The KubeVela Authors.

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

package bcode

var (
	// ErrSensitiveConfig means the config can not be read
	ErrSensitiveConfig = NewBcode(400, 16001, "the config is sensitive")

	// ErrNoConfigOrTarget means there is no target or config when creating the distribution.
	ErrNoConfigOrTarget = NewBcode(400, 16002, "you must specify the config name and destination to distribute")

	// ErrConfigExist means the config does exist
	ErrConfigExist = NewBcode(400, 16003, "the config name does exist")

	// ErrChangeTemplate the template of the config can not be changed
	ErrChangeTemplate = NewBcode(400, 16004, "the template of the config can not be changed")

	// ErrTemplateNotFound means the template does not exist
	ErrTemplateNotFound = NewBcode(404, 16005, "the template does not exist")

	// ErrConfigNotFound means the config does not exist
	ErrConfigNotFound = NewBcode(404, 16006, "the config does not exist")

	// ErrNotFoundDistribution means the distribution does not exist
	ErrNotFoundDistribution = NewBcode(404, 16007, "the distribution does not exist")

	// ErrChangeSecretType the secret type of the config can not be changed
	ErrChangeSecretType = NewBcode(400, 16008, "the secret type of the config can not be changed")
)
