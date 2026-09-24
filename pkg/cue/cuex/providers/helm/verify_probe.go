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

// DIAGNOSTIC ONLY. This file exists to verify that cuex's Resolve walker
// fires every #do/#provider marker in a helmchart CUE template, regardless
// of whether the marker's $returns are referenced by anything downstream.
//
// Remove this file (and the probes referencing it in helmchart.cue +
// the registration in helm.go) once the experiment is done. It adds an
// INFO log line per cuex evaluation per probe per reconcile.

package helm

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"

	"k8s.io/klog/v2"

	providers "github.com/kubevela/pkg/cue/cuex/providers"
)

// VerifyFnCallParams captures enough context to attribute a probe firing to
// a specific Application + component + call site in helmchart.cue.
type VerifyFnCallParams struct {
	AppName       string `json:"appName"`
	ComponentName string `json:"componentName"`
	// CallerHint is set by the probe site in helmchart.cue. Distinct values
	// let us tell apart firings from different paths in the same template
	// (root probe vs nested probe vs probe inside output, etc.).
	CallerHint string `json:"callerHint,omitempty"`
}

// VerifyFnCallReturns are static. Nothing in the templates reads them; the
// probe is observed via klog output, not by CUE consumers.
type VerifyFnCallReturns struct {
	Verified  bool   `json:"verified"`
	Timestamp string `json:"timestamp"`
}

// verifyFnCallCount is a process-wide counter so the operator can spot
// reconcile-rate explosions ("probe fired 1000 times in 30 seconds means
// the controller is in a hot loop").
var verifyFnCallCount uint64

// VerifyFnCall is a no-op cuex provider function used purely for
// observability. It logs every invocation at INFO so the line is visible
// without needing klog -v=N flags.
//
// What this proves: if the probe fires from a CUE path that nothing
// downstream consumes, then cuex's Resolve walker is exhaustive (it
// evaluates every #do/#provider field, not only the ones whose $returns
// are needed by something else).
func VerifyFnCall(ctx context.Context, params *providers.Params[VerifyFnCallParams]) (*providers.Returns[VerifyFnCallReturns], error) {
	count := atomic.AddUint64(&verifyFnCallCount, 1)

	// Identify which Go function called us. Should always be cuex's
	// Resolve loop (Compiler.Resolve in github.com/kubevela/pkg). If the
	// caller name is anything else, we have a second invocation site we
	// did not expect and want to surface.
	pc, _, _, _ := runtime.Caller(1)
	callerName := runtime.FuncForPC(pc).Name()

	klog.InfoS("[CUEX-PROBE] verifyFnCall fired",
		"app", params.Params.AppName,
		"component", params.Params.ComponentName,
		"hint", params.Params.CallerHint,
		"isDryRun", isDryRun(ctx),
		"goCallerOfProvider", callerName,
		"countSinceProcessStart", count,
	)

	return &providers.Returns[VerifyFnCallReturns]{
		Returns: VerifyFnCallReturns{
			Verified:  true,
			Timestamp: time.Now().Format(time.RFC3339Nano),
		},
	}, nil
}
