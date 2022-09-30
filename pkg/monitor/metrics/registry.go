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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"k8s.io/component-base/metrics/legacyregistry"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// multiRegistererGatherer mixes multiple gatherer
type multiRegistererGatherer struct {
	prometheus.Registerer
	gatherers []prometheus.Gatherer
}

func (in *multiRegistererGatherer) Gather() ([]*dto.MetricFamily, error) {
	var mfs []*dto.MetricFamily
	for _, g := range in.gatherers {
		mf, err := g.Gather()
		if err != nil {
			return nil, err
		}
		mfs = append(mfs, mf...)
	}
	return mfs, nil
}

func init() {
	r := metrics.Registry
	// merge multiple registry
	metrics.Registry = &multiRegistererGatherer{
		Registerer: r,
		gatherers:  []prometheus.Gatherer{r, legacyregistry.DefaultGatherer},
	}
}
