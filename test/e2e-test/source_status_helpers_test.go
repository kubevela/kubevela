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

package controllers_test

import (
	"encoding/json"
	"fmt"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// Source consumption is reported once per Application in status.sources[], not
// per component. These read that shape so a spec can ask "what did this binding
// resolve to" without walking it by hand each time.

// sourceStatus finds a declared binding in the Application-level report.
func sourceStatus(app *v1beta1.Application, name string) (common.ApplicationSourceStatus, error) {
	for _, src := range app.Status.Sources {
		if src.Name == name {
			return src, nil
		}
	}
	return common.ApplicationSourceStatus{}, fmt.Errorf("source %q missing from status.sources", name)
}

// storageKeyOf returns the key of the single entry backing a binding.
//
// Errors when a binding has several, rather than picking one: that only happens
// when the cache key varies, and a spec asserting on "the" entry would be
// asserting on whichever came first.
func storageKeyOf(app *v1beta1.Application, name string) (string, error) {
	src, err := sourceStatus(app, name)
	if err != nil {
		return "", err
	}
	switch len(src.Resolutions) {
	case 0:
		return "", fmt.Errorf("source %q has resolved no entries yet", name)
	case 1:
		return src.Resolutions[0].StorageKey, nil
	default:
		return "", fmt.Errorf("source %q has %d entries; assert on one explicitly", name, len(src.Resolutions))
	}
}

// consumedValue returns what a reader took from one attribute of a source,
// rendered as it appears in status. Redacted attributes come back as "***", so a
// spec can assert redaction without knowing which consumer made the read.
func consumedValue(app *v1beta1.Application, sourceName, attr string) (string, error) {
	src, err := sourceStatus(app, sourceName)
	if err != nil {
		return "", err
	}
	for _, by := range src.ConsumedBy {
		for _, v := range by.Values {
			if v.SourceAttr != attr || v.Value == nil {
				continue
			}
			var s string
			if err := json.Unmarshal(v.Value.Raw, &s); err == nil {
				return s, nil
			}
			return string(v.Value.Raw), nil
		}
	}
	return "", fmt.Errorf("no consumer of source %q recorded a read of %q", sourceName, attr)
}
