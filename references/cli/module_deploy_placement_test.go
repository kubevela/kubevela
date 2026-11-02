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

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// dryRunPlacement runs deploy --dry-run with the given placement flags and
// returns the placement property on the printed type: module component, or
// nil when the component carries none.
func dryRunPlacement(o *moduleDeployOptions) map[string]interface{} {
	var out bytes.Buffer
	o.module, o.registry, o.namespace, o.dryRun = "placement-kit", "catalog", velatypes.DefaultKubeVelaNS, true
	o.fetch = func(_ context.Context, _, _, _ string) (*pkgmodule.Module, error) { return stubModule(), nil }
	Expect(o.run(context.Background(), moduleDeployClient(GinkgoTB(), "catalog"), &out)).To(Succeed())

	var printed v1beta1.Application
	Expect(yaml.Unmarshal(out.Bytes(), &printed)).To(Succeed())
	var props map[string]interface{}
	Expect(json.Unmarshal(printed.Spec.Components[0].Properties.Raw, &props)).To(Succeed())
	place, _ := props["placement"].(map[string]interface{})
	return place
}

// settledOwnedApp returns the deploy Application, running, and an owned
// Application with every tier healthy, in the given phase and generation,
// with the given workflow steps.
func settledOwnedApp(phase oamcommon.ApplicationPhase, generation, observed int64, steps []workflowv1alpha1.StepStatus) []client.Object {
	objs := moduleApps(oamcommon.ApplicationRunning, moduleTiers, healthyTierServices())
	owned := objs[1].(*v1beta1.Application)
	owned.Generation = generation
	owned.Status.Phase = phase
	owned.Status.ObservedGeneration = observed
	if steps != nil {
		owned.Status.Workflow = &oamcommon.WorkflowStatus{}
		for _, st := range steps {
			owned.Status.Workflow.Steps = append(owned.Status.Workflow.Steps, workflowv1alpha1.WorkflowStepStatus{StepStatus: st})
		}
	}
	return objs
}

// waitOn runs waitForModule against the given objects and returns its error
// and output.
func waitOn(objs []client.Object, timeout time.Duration) (error, string) {
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(objs...).Build()
	var out bytes.Buffer
	o := &moduleDeployOptions{module: "s3", namespace: velatypes.DefaultKubeVelaNS, timeout: timeout, pollInterval: time.Millisecond}
	err := o.waitForModule(context.Background(), cli, &out)
	return err, out.String()
}

var _ = Describe("vela module deploy placement flags", func() {
	DescribeTable("prints the placement property on --dry-run",
		func(opts moduleDeployOptions, want map[string]interface{}) {
			Expect(dryRunPlacement(&opts)).To(Equal(want))
		},
		Entry("no placement flags", moduleDeployOptions{}, nil),
		Entry("clusters", moduleDeployOptions{clusters: []string{"spoke-a", "spoke-b"}},
			map[string]interface{}{"clusters": []interface{}{"spoke-a", "spoke-b"}}),
		Entry("cluster selector", moduleDeployOptions{clusterSelector: map[string]string{"tier": "runtime"}},
			map[string]interface{}{"clusterLabelSelector": map[string]interface{}{"tier": "runtime"}}),
		Entry("allow empty with a selector", moduleDeployOptions{clusterSelector: map[string]string{"tier": "runtime"}, allowEmpty: true},
			map[string]interface{}{"clusterLabelSelector": map[string]interface{}{"tier": "runtime"}, "allowEmpty": true}),
	)

	DescribeTable("rejects bad flags before the module is fetched",
		func(opts moduleDeployOptions, want string) {
			cli := moduleDeployClient(GinkgoTB(), "catalog")
			opts.module, opts.registry, opts.namespace = "placement-kit", "catalog", velatypes.DefaultKubeVelaNS
			opts.fetch = func(_ context.Context, _, _, _ string) (*pkgmodule.Module, error) {
				Fail("a bad placement must fail before the module is fetched")
				return nil, nil
			}

			err := opts.run(context.Background(), cli, &bytes.Buffer{})

			Expect(err).To(MatchError(want))
			Expect(countApplications(GinkgoTB(), cli)).To(Equal(0), "nothing may be applied when validation fails")
		},
		Entry("clusters and selector together", moduleDeployOptions{clusters: []string{"spoke"}, clusterSelector: map[string]string{"tier": "runtime"}},
			"--clusters and --cluster-selector cannot be used together"),
		Entry("allow empty alone", moduleDeployOptions{allowEmpty: true},
			"--allow-empty needs --clusters or --cluster-selector"),
		Entry("blank cluster name", moduleDeployOptions{clusters: []string{"spoke", ""}},
			"placement clusters must not contain an empty name"),
	)

	It("registers the three flags with their types", func() {
		cmd := NewModuleDeployCommand(common.Args{}, cmdutil.IOStreams{Out: &bytes.Buffer{}})
		Expect(cmd.ParseFlags([]string{
			"--clusters", "spoke-a,spoke-b",
			"--cluster-selector", "tier=runtime,region=west",
			"--allow-empty",
		})).To(Succeed())

		clusters, err := cmd.Flags().GetStringSlice("clusters")
		Expect(err).NotTo(HaveOccurred())
		Expect(clusters).To(Equal([]string{"spoke-a", "spoke-b"}))
		selector, err := cmd.Flags().GetStringToString("cluster-selector")
		Expect(err).NotTo(HaveOccurred())
		Expect(selector).To(Equal(map[string]string{"tier": "runtime", "region": "west"}))
		allowEmpty, err := cmd.Flags().GetBool("allow-empty")
		Expect(err).NotTo(HaveOccurred())
		Expect(allowEmpty).To(BeTrue())
	})
})

var _ = Describe("vela module deploy waiting on a placed module", func() {
	It("reports a tier as unhealthy when one of its clusters is", func() {
		tier, message := firstUnhealthyTier([]string{"s3-xrd", "s3-v1-comp", "s3-v1-defs"}, []oamcommon.ApplicationComponentStatus{
			{Name: "s3-xrd", Cluster: "spoke-2", Healthy: false, Message: "crd not established"},
			{Name: "s3-xrd", Cluster: "spoke", Healthy: true},
			{Name: "s3-v1-comp", Healthy: true},
			{Name: "s3-v1-defs", Healthy: true},
		}, false)
		Expect(tier).To(Equal("s3-xrd"))
		Expect(message).To(ContainSubstring("crd not established"))
	})

	// A runtime step whose selector matches no cluster fails while the hub
	// tiers are healthy: deploy reports the step, not a success.
	It("waits for the owned workflow and names the failed step", func() {
		err, out := waitOn(settledOwnedApp(oamcommon.ApplicationRunningWorkflow, 1, 1, []workflowv1alpha1.StepStatus{
			{Name: "deploy-module-hub", Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded},
			{Name: "deploy-module-runtime", Phase: workflowv1alpha1.WorkflowStepPhaseFailed, Message: "failed to find any cluster matches given labels"},
		}), 30*time.Millisecond)
		Expect(err).To(MatchError(ContainSubstring("deploy-module-runtime")))
		Expect(err.Error()).To(ContainSubstring("failed to find any cluster matches given labels"))
		Expect(out).NotTo(ContainSubstring("is installed"))
	})

	It("does not count a status from before the latest change", func() {
		err, out := waitOn(settledOwnedApp(oamcommon.ApplicationRunning, 2, 1, nil), 30*time.Millisecond)
		Expect(err).To(HaveOccurred())
		Expect(out).NotTo(ContainSubstring("is installed"))
	})

	// With --allow-empty and no matching cluster, the runtime tiers never
	// report a status; once the owned Application has finished they show as
	// not placed instead of holding the wait.
	It("accepts tiers that went to no cluster", func() {
		objs := settledOwnedApp(oamcommon.ApplicationRunning, 3, 3, nil)
		owned := objs[1].(*v1beta1.Application)
		owned.Status.Services = owned.Status.Services[1:]
		err, out := waitOn(objs, time.Second)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(ContainSubstring("Not placed"))
		Expect(out).To(ContainSubstring("is installed"))
	})

	It("stops at once on a terminal owned workflow", func() {
		start := time.Now()
		err, _ := waitOn(settledOwnedApp(oamcommon.ApplicationWorkflowFailed, 4, 4, []workflowv1alpha1.StepStatus{
			{Name: "deploy-module-runtime", Phase: workflowv1alpha1.WorkflowStepPhaseFailed, Message: "failed to get cluster spoke-9"},
		}), time.Minute)
		Expect(err).To(MatchError(ContainSubstring(string(oamcommon.ApplicationWorkflowFailed))))
		Expect(err.Error()).To(ContainSubstring("failed to get cluster spoke-9"))
		Expect(time.Since(start)).To(BeNumerically("<", 5*time.Second), "a terminal owned phase must not wait out the timeout")
	})

	It("ignores a failed step from before the latest change", func() {
		err, out := waitOn(settledOwnedApp(oamcommon.ApplicationWorkflowFailed, 2, 1, []workflowv1alpha1.StepStatus{
			{Name: "deploy-module-runtime", Phase: workflowv1alpha1.WorkflowStepPhaseFailed, Message: "stale failure"},
		}), 30*time.Millisecond)
		Expect(err).To(MatchError(ContainSubstring("timed out")))
		Expect(err.Error()).NotTo(ContainSubstring("stale failure"))
		Expect(out).NotTo(ContainSubstring("stale failure"))
	})

	// Until the deploy Application has reconciled its new spec, the owned
	// Application still carries the previous request's result, even at its own
	// current generation.
	It("ignores the previous request's failure on a redeploy", func() {
		objs := settledOwnedApp(oamcommon.ApplicationWorkflowFailed, 4, 4, []workflowv1alpha1.StepStatus{
			{Name: "deploy-module-runtime", Phase: workflowv1alpha1.WorkflowStepPhaseFailed, Message: "previous request failed"},
		})
		deployApp := objs[0].(*v1beta1.Application)
		deployApp.Generation = 2
		deployApp.Status.ObservedGeneration = 2
		deployApp.Status.LatestRevision = &oamcommon.Revision{Name: "module-s3-deploy-v2"}
		err, out := waitOn(objs, 30*time.Millisecond)
		Expect(err).To(MatchError(ContainSubstring("timed out")))
		Expect(err.Error()).NotTo(ContainSubstring("previous request failed"))
		Expect(out).NotTo(ContainSubstring("previous request failed"))
	})

	// The module component's health is the owned Application's health, so the
	// deploy Application waits in runningWorkflow while the runtime step fails.
	It("reports the failed step while the deploy Application waits", func() {
		objs := settledOwnedApp(oamcommon.ApplicationRunningWorkflow, 1, 1, []workflowv1alpha1.StepStatus{
			{Name: "deploy-module-hub", Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded},
			{Name: "deploy-module-runtime", Phase: workflowv1alpha1.WorkflowStepPhaseFailed, Message: "failed to find any cluster matches given labels"},
		})
		objs[0].(*v1beta1.Application).Status.Phase = oamcommon.ApplicationRunningWorkflow
		err, out := waitOn(objs, 30*time.Millisecond)
		Expect(err).To(MatchError(ContainSubstring("deploy-module-runtime failed")))
		Expect(out).To(ContainSubstring("Step deploy-module-runtime failed"))
	})
})
