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

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestConfigMigrationCounterRegisteredOnce(t *testing.T) {
	err := metrics.Registry.Register(ConfigMigrationCounter)
	var already prometheus.AlreadyRegisteredError
	require.ErrorAs(t, err, &already, "init should have registered the counter with the controller-runtime registry")
}

func TestConfigMigrationCounterResultLabels(t *testing.T) {
	for _, result := range []string{"adopted", "skipped", "conflict", "failed"} {
		before := testutil.ToFloat64(ConfigMigrationCounter.WithLabelValues(result))
		ConfigMigrationCounter.WithLabelValues(result).Add(2)
		require.Equal(t, before+2, testutil.ToFloat64(ConfigMigrationCounter.WithLabelValues(result)), result)
	}
	require.Equal(t, 4, testutil.CollectAndCount(ConfigMigrationCounter, "kubevela_config_migration_total"))
}
