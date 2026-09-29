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

package sources

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A DefinitionError built outside the resolver still reads as its entries.
func TestDefinitionErrorFallsBackToItsEntries(t *testing.T) {
	require.Equal(t, "region is required; tier: conflicting values",
		(&DefinitionError{User: []string{"region is required"}, Schema: []string{"tier: conflicting values"}}).Error())
	require.Equal(t, "source definition refused", (&DefinitionError{}).Error())
	require.Equal(t, "set", (&DefinitionError{message: "set", User: []string{"x"}}).Error())
}
