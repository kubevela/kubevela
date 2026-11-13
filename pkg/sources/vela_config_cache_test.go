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
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/velaconfig"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/cachekey"
)

// A read a consumer may make is checked once and then cached, so a vela-config entry must not
// be shared with a consumer in another namespace, which may not be allowed the read.
func TestVelaConfigIsCachedPerConsumerNamespace(t *testing.T) {
	src, err := os.ReadFile("../../vela-templates/definitions/internal/source/vela-config.cue")
	require.NoError(t, err)
	rules, err := cachekey.LoadRules()
	require.NoError(t, err)
	dims, err := cachekey.Infer(string(src), rules)
	require.NoError(t, err)
	var fields []string
	for _, d := range dims {
		fields = append(fields, d.String())
	}
	require.Contains(t, fields, "namespace")
}

// The resolver records the consumer's namespace for providers that read on its behalf.
func TestSourceResolverRecordsTheConsumerNamespace(t *testing.T) {
	r := newSourceResolver(context.Background(), map[string]interface{}{velaprocess.ContextNamespace: "team-a"}, "component", sourceInputs{})
	ns, ok := velaconfig.ConsumerNamespaceFrom(r.goCtx)
	require.True(t, ok)
	require.Equal(t, "team-a", ns)
}

// A consumer the controller recorded stands: a component placed in another namespace still reads
// for its Application's.
func TestSourceResolverKeepsARecordedConsumerNamespace(t *testing.T) {
	ctx := velaconfig.WithConsumerNamespace(context.Background(), "team-a")
	r := newSourceResolver(ctx, map[string]interface{}{velaprocess.ContextNamespace: "team-b"}, "component", sourceInputs{})
	ns, ok := velaconfig.ConsumerNamespaceFrom(r.goCtx)
	require.True(t, ok)
	require.Equal(t, "team-a", ns)
}
