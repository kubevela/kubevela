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

package resourcetracker

import (
	"sync/atomic"

	"github.com/kubevela/pkg/util/compression"
)

// Config holds the behaviour the host binary decides for this package. Its functions are
// called on every use, so a host can back them with settings that change at runtime. A
// nil function takes the default documented on it, which matches KubeVela's default.
type Config struct {
	// Compression returns how new ResourceTrackers compress their records; "" leaves them
	// uncompressed. Default "" (KubeVela: ZstdResourceTracker, else GzipResourceTracker).
	Compression func() compression.Type
	// OnList is called whenever an owner's trackers are listed, with the owner kind as that
	// kind spells it ("Application", "Component"). Default none (KubeVela: a counter metric).
	OnList func(kind string)
	// KindLabels returns the labels an owner kind puts on its resources besides
	// owner.oam.dev/*, so a resource handed to the next sharer is labelled as that kind
	// labels its own. Default: none (KubeVela: app.oam.dev/* for Applications).
	KindLabels func(kind, namespace, name string) map[string]string
}

var config atomic.Pointer[Config]

// Configure sets this package's Config for the process. Hosts call it at start-up.
func Configure(c Config) {
	config.Store(&c)
}

// CurrentConfig returns the Config in effect (the zero Config if Configure was never called).
func CurrentConfig() Config {
	if c := config.Load(); c != nil {
		return *c
	}
	return Config{}
}

func compressionType() compression.Type {
	if f := CurrentConfig().Compression; f != nil {
		return f()
	}
	return ""
}

// OnList reports a listing of an owner kind's trackers to the configured OnList.
func OnList(kind string) {
	if f := CurrentConfig().OnList; f != nil {
		f(kind)
	}
}
