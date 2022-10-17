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

package docgen

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/olekukonko/tablewriter"
)

// GenerateConsoleDocument generate the document shown on the console.
func GenerateConsoleDocument(title string, schema *openapi3.Schema) (string, error) {
	var buffer = &bytes.Buffer{}
	var printSubProperties []*openapi3.Schema
	if len(schema.Properties) > 0 {
		var propertiesTable = tablewriter.NewWriter(buffer)
		propertiesTable.SetHeader([]string{"NAME", "TYPE", "DESCRIPTION", "REQUIRED", "OPTIONS", "DEFAULT"})
		for key, subSchema := range schema.Properties {
			name := subSchema.Value.Title
			if title != "" {
				name = fmt.Sprintf("(%s).%s", title, name)
			}
			defaultValue := fmt.Sprintf("%v", subSchema.Value.Default)
			if subSchema.Value.Default == nil {
				defaultValue = ""
			}
			var options = ""
			for _, enum := range subSchema.Value.Enum {
				options += fmt.Sprintf("%v", enum)
			}
			propertiesTable.Append([]string{
				name,
				subSchema.Value.Type,
				subSchema.Value.Description,
				fmt.Sprintf("%t", strings.Contains(strings.Join(schema.Required, "/"), subSchema.Value.Title)),
				options,
				defaultValue,
			})
			if len(subSchema.Value.Properties) > 0 {
				printSubProperties = append(printSubProperties, schema.Properties[key].Value)
			}
		}
		buffer.WriteString(title + "\n")
		propertiesTable.Render()
	}

	for _, sub := range printSubProperties {
		next := strings.Join([]string{title, sub.Title}, ".")
		if title == "" {
			next = sub.Title
		}
		re, err := GenerateConsoleDocument(next, sub)
		if err != nil {
			return "", err
		}
		buffer.WriteString(re)
	}

	return buffer.String(), nil
}
