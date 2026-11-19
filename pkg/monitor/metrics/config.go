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
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ConfigMigrationCounter counts the rows of the startup legacy config migration
// sweep by result: adopted, skipped, conflict or failed. The sweep adds the final
// attempt's report, so a restart on a fully migrated cluster adds to skipped only,
// which is how an operator sees that the sweep ran and found nothing to do.
var ConfigMigrationCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "kubevela_config_migration_total",
	Help: "Legacy config templates and configs seen by the startup migration sweep, by result.",
}, []string{"result"})

func init() {
	if err := metrics.Registry.Register(ConfigMigrationCounter); err != nil {
		klog.Error(err)
	}
}
