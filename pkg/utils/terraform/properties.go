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

package terraform

const (
	// TerraformWriteConnectionSecretToRefName is the name for Terraform WriteConnectionSecretToRef
	TerraformWriteConnectionSecretToRefName = "writeConnectionSecretToRef"
	// TerraformWriteConnectionSecretToRefType is the type for Terraform WriteConnectionSecretToRef
	TerraformWriteConnectionSecretToRefType = "[writeConnectionSecretToRef](#writeConnectionSecretToRef)"
	// TerraformWriteConnectionSecretToRefDescription is the description for Terraform WriteConnectionSecretToRef
	TerraformWriteConnectionSecretToRefDescription = "The secret which the cloud resource connection will be written to"
	// TerraformSecretNameDescription is the description for the name for Terraform Secret
	TerraformSecretNameDescription = "The secret name which the cloud resource connection will be written to"
	// TerraformSecretNamespaceDescription is the description for the namespace for Terraform Secret
	TerraformSecretNamespaceDescription = "The secret namespace which the cloud resource connection will be written to"
)
