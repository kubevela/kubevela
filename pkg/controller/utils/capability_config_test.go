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

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTerraformConfigurationFromRemote(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	main := `
variable "aaa" {
	type = list(object({
		type = string
		sourceArn = string
		config = string
	}))
	default = []
}`
	vars := `
variable "aws_az" {
  description = "AWS Availability Zone"
  type        = string
}`
	t.Run("reads main.tf at the module's root path", func(t *testing.T) {
		conf, err := GetTerraformConfigurationFromRemote("valid", moduleRepo(t, map[string]string{"unittest/main.tf": main}, nil), "unittest/", nil)
		assert.NoError(t, err)
		assert.Equal(t, main, conf)
	})
	t.Run("reads variables.tf at a nested path", func(t *testing.T) {
		conf, err := GetTerraformConfigurationFromRemote("aws-subnet", moduleRepo(t, map[string]string{"unittest/aws/subnet/variables.tf": vars}, nil), "unittest/aws/subnet", nil)
		assert.NoError(t, err)
		assert.Equal(t, vars, conf)
	})
}
