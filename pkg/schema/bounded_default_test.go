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

package schema

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// The controller stores a definition's schema through ParsePropertiesToSchema;
// a number with a default and a bound failed it with "unsupported op for
// number &" and left the definition Synced=False (#5767).
func TestParsePropertiesToSchemaBoundedDefault(t *testing.T) {
	sch, err := ParsePropertiesToSchema(context.Background(), `
parameter: {
	// +usage=Image to run
	image: string
	// +usage=Number of replicas, 1 to 5
	replicas: *1 | int & >=1 & <=5
	// +usage=Seconds to wait before killing the container
	terminationGracePeriodSeconds: *30 | >=1 & <=3599 & int
}
`)
	require.NoError(t, err)
	FixOpenAPISchema("", sch)

	replicas := sch.Properties["replicas"].Value
	require.Equal(t, float64(1), replicas.Default)
	require.Equal(t, float64(1), *replicas.Min)
	require.Equal(t, float64(5), *replicas.Max)
	require.Equal(t, "Number of replicas, 1 to 5", replicas.Description, "the +usage prefix is removed as before")

	grace := sch.Properties["terminationGracePeriodSeconds"].Value
	require.Equal(t, float64(30), grace.Default)
	require.Equal(t, float64(3599), *grace.Max)
}
