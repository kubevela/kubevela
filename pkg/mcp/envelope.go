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

package mcp

import (
	"context"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"

	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
)

const (
	// SourceLive marks data read directly from the API server.
	SourceLive = "live"
	// SourceRendered marks output of a dry-run or diff, never applied.
	SourceRendered = "rendered"
	// SourceCached marks data served from a cache; only definition schemas
	// may be, and they carry their revision so staleness is visible.
	SourceCached = "cached"

	// platformCacheTTL bounds how long version and gate information is
	// reused between tool calls. It is cheap to refresh and must not go
	// stale across an upgrade during a long session.
	platformCacheTTL = 30 * time.Second
)

// Meta is the envelope every tool response carries. It answers "against
// what, and when" so a reader can judge a result without trusting it.
type Meta struct {
	// VelaCLI is the version of the vela binary serving this session.
	VelaCLI string `json:"velaCLI"`
	// VelaCore is the controller version found on the cluster, or "unknown".
	VelaCore string `json:"velaCore"`
	// Kubernetes maps a cluster name to its API server version. This release
	// reports the local cluster only.
	Kubernetes map[string]string `json:"kubernetes"`
	// FeatureGates lists the controller feature gates read from vela-core's
	// arguments. Absent gates are at their defaults.
	FeatureGates map[string]bool `json:"featureGates,omitempty"`
	// ObservedAt is when the data in this response was read.
	ObservedAt time.Time `json:"observedAt"`
	// Source is one of SourceLive, SourceRendered or SourceCached.
	Source string `json:"source"`
	// Truncated is set when a size or line budget cut the data.
	Truncated bool `json:"truncated"`
	// Warnings carries version skew and similar findings that every tool
	// repeats so they are not missed.
	Warnings []string `json:"warnings,omitempty"`
}

// deps holds what handlers need: the factory for clients and config, the
// options, and a short-lived cache of platform information so the envelope
// does not cost a Deployment read on every call.
type deps struct {
	f    velacmd.Factory
	opts Options

	mu          sync.Mutex
	platform    *platformInfo
	platformAt  time.Time
	platformErr error
	cs          kubernetes.Interface
}

// newDeps completes the per-handler defaults that need the deps themselves,
// which Options.complete cannot do.
func newDeps(f velacmd.Factory, opts Options) *deps {
	d := &deps{f: f, opts: opts}
	if d.opts.PodLogs == nil {
		d.opts.PodLogs = d.streamPodLogs
	}
	return d
}

// meta builds the envelope for a live response. Plan tools set Source to
// SourceRendered on the result they return. Platform lookups are cached for
// platformCacheTTL; a failed lookup is reported inside the envelope rather
// than failing the tool, because an answer with "vela-core: unknown" is
// still more useful than no answer.
func (d *deps) meta(ctx context.Context) Meta {
	info, err := d.cachedPlatform(ctx)
	m := Meta{
		VelaCLI:    info.VelaCLI,
		VelaCore:   info.VelaCore,
		Kubernetes: info.Kubernetes,
		ObservedAt: time.Now().UTC(),
		Source:     SourceLive,
	}
	if len(info.FeatureGates) > 0 {
		m.FeatureGates = info.FeatureGates
	}
	m.Warnings = append(m.Warnings, info.Warnings...)
	if err != nil {
		m.Warnings = append(m.Warnings, "platform information incomplete: "+redactLine(err.Error()))
	}
	return m
}

func (d *deps) cachedPlatform(ctx context.Context) (platformInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.platform != nil && time.Since(d.platformAt) < platformCacheTTL {
		return *d.platform, d.platformErr
	}
	info, err := d.readPlatform(ctx)
	d.platform, d.platformAt, d.platformErr = &info, time.Now(), err
	return info, err
}
