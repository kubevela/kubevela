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

// Package ginkgotest runs *_test.cue definition tests as Ginkgo specs.
package ginkgotest

import (
	"fmt"
	"slices"
	"strings"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	"github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"

	"github.com/oam-dev/kubevela/pkg/definition/cuetest"
)

// DescribeDefinitions registers an ordered container for path, holding a
// container per test file under it and a spec per case, labelled with the
// case's labels and located at the case, so
// Ginkgo's --label-filter, --focus and --focus-file select them. A @pending
// case is a Pending spec, and an @upgrade case that passes once upgraded
// carries its reason as a report entry. Definitions are tested as written
// unless WithUpgrades is given. Call it at
// package level: `var _ = ginkgotest.DescribeDefinitions("./defs")`.
//
// A path's cases run in order on one process, which starts a test cluster
// for its step and source cases and stops it after them; under ginkgo -p,
// separate paths still run on separate processes.
//
// Renders read CueX's compiler singletons, which with external packages on
// load them from the kubeconfig's cluster, so the suite's Test function turns
// that off before RunSpecs: cuex.EnableExternalPackageForDefaultCompiler =
// false. The switch is process-wide, so it is the suite's to set.
func DescribeDefinitions(path string, opts ...Option) bool {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if o.cluster != nil {
		if err := mergeClusterOptions(&cuetest.ExecClusterOptions, *o.cluster); err != nil {
			return ginkgo.It("configures the test cluster for "+path, func() { ginkgo.Fail(err.Error()) })
		}
	}

	// Loading reads CueX's compiler singletons, so it waits for RunSpecs to
	// build the tree, after the suite's Test function has turned their
	// external packages off.
	//
	// Ordered, so the container's cases run on one process, which stops the
	// cluster it started for them: a suite node runs on process 1 only, so
	// under ginkgo -p would leave the other processes' clusters running.
	return ginkgo.Describe(path, ginkgo.Ordered, func() {
		ginkgo.AfterAll(func() {
			_ = cuetest.StopExecCluster()
		})
		describeSuites(path, o)
	})
}

func describeSuites(path string, o options) {
	suites, err := cuetest.Load(path)
	if err != nil {
		ginkgo.It("loads "+path, func() { ginkgo.Fail(err.Error()) })
		return
	}
	for _, s := range suites {
		if s.Err != nil {
			ginkgo.It("loads "+s.File, types.CodeLocation{FileName: s.File, LineNumber: 1}, func() { ginkgo.Fail(s.Err.Error()) })
			continue
		}
		var decorators []any
		if len(s.Before)+len(s.After) > 0 {
			// BeforeAll and AfterAll need an ordered container; @beforeEach
			// and @afterEach run inside each case.
			decorators = append(decorators, ginkgo.Ordered)
		}
		ginkgo.Describe(s.File, append(decorators, func() {
			if len(s.Before) > 0 {
				ginkgo.BeforeAll(func() {
					if err := s.RunBefore(); err != nil {
						ginkgo.Fail(err.Error())
					}
				})
			}
			if len(s.Before)+len(s.After) > 0 {
				// Also deletes what the file's hooks created.
				ginkgo.AfterAll(func() {
					if err := s.RunAfter(); err != nil {
						ginkgo.Fail(err.Error())
					}
				})
			}
			for _, c := range s.Cases {
				args := []any{types.CodeLocation{FileName: c.File, LineNumber: c.Line}, ginkgo.Label(c.Labels...)}
				if c.Pending {
					args = append(args, ginkgo.Pending)
				}
				args = append(args, func() {
					m := &passMatcher{opts: o.run}
					gomega.Expect(c).To(m)
					if m.outcome.Upgraded {
						ginkgo.AddReportEntry("@upgrade", c.Upgrade.Reason)
					}
				})
				ginkgo.It(fmt.Sprintf("%s / %s", c.Subject.Name, c.Name), args...)
			}
		})...)
	}
}

// mergeClusterOptions adds one call's cluster options to the shared ones: the
// cluster starts once, after every call has registered, so each call's CRDs
// are installed, and two calls naming different envtest binaries conflict.
func mergeClusterOptions(into *cuetest.ClusterOptions, add cuetest.ClusterOptions) error {
	if add.Assets != "" {
		if into.Assets != "" && into.Assets != add.Assets {
			return fmt.Errorf("envtest assets %s conflict with %s, given by another DescribeDefinitions: a suite has one test cluster", add.Assets, into.Assets)
		}
		into.Assets = add.Assets
	}
	for _, crd := range add.CRDs {
		if !slices.Contains(into.CRDs, crd) {
			into.CRDs = append(into.CRDs, crd)
		}
	}
	return nil
}

// Option configures DescribeDefinitions.
type Option func(*options)

type options struct {
	run     cuetest.RunOptions
	cluster *cuetest.ClusterOptions
}

// WithEnvtestAssets is the directory holding etcd and kube-apiserver for
// workflow step cases, in place of KUBEBUILDER_ASSETS.
func WithEnvtestAssets(dir string) Option {
	return func(o *options) {
		if o.cluster == nil {
			o.cluster = &cuetest.ClusterOptions{}
		}
		o.cluster.Assets = dir
	}
}

// WithCRDs are CRD files or directories to install for workflow step cases,
// in place of the vela-core chart's.
func WithCRDs(paths ...string) Option {
	return func(o *options) {
		if o.cluster == nil {
			o.cluster = &cuetest.ClusterOptions{}
		}
		o.cluster.CRDs = append(o.cluster.CRDs, paths...)
	}
}

// WithUpgrades runs definitions through KubeVela's CUE upgrader, with every
// rewrite pass on. Without it they are tested as written.
func WithUpgrades() Option {
	return func(o *options) { o.run.Upgrades = true }
}

// WithFailOnUpgrade fails @upgrade cases that fail as written, instead of
// retrying them upgraded and reporting the marker's reason.
func WithFailOnUpgrade() Option {
	return func(o *options) { o.run.FailOnUpgrade = true }
}

// Pass succeeds when a case, evaluated as written with its @upgrade marker
// honoured, produces what it expects.
func Pass() gomegatypes.GomegaMatcher {
	return &passMatcher{}
}

type passMatcher struct {
	opts    cuetest.RunOptions
	at      string
	outcome cuetest.Outcome
}

func (m *passMatcher) Match(actual any) (bool, error) {
	c, ok := actual.(*cuetest.Case)
	if !ok {
		return false, fmt.Errorf("Pass expects a *cuetest.Case, got %T", actual)
	}
	m.at = fmt.Sprintf("%s:%d", c.File, c.Line)
	m.outcome = c.Evaluate(m.opts)
	return len(m.outcome.Failures) == 0, nil
}

func (m *passMatcher) FailureMessage(any) string {
	return m.at + ": definition test failed:\n  " + strings.Join(m.outcome.Failures, "\n  ")
}

func (m *passMatcher) NegatedFailureMessage(any) string {
	return "expected the definition test to fail, but it passed"
}
