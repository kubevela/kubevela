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

package propexpr

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// What survived the expression engine. A source's schema is CUE and always will
// be, so walking one is still this package's job; only evaluating an expression
// moved out.

// newContext returns the CUE context an evaluation runs in.
//
// One per call: a cue.Value belongs to the context that made it, so the scope
// and the expression have to share one.
func newContext() *cue.Context { return cuecontext.New() }
