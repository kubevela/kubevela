/*
Copyright 2021 The KubeVela Authors.

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
	"context"
	"fmt"
	"io"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// VelaRuntimeStatus enums vela-core runtime status
type VelaRuntimeStatus int

// Enums of VelaRuntimeStatus
const (
	NotFound VelaRuntimeStatus = iota
	Pending
	Ready
	Error
)

type infoCmd struct {
	out io.Writer
}

// SystemCommandGroup creates `system` command and its nested children command
func SystemCommandGroup(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "system",
		Short:  "System management utilities",
		Long:   "System management utilities.",
		Hidden: true,
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.AddCommand(NewAdminInfoCommand(ioStream))
	return cmd
}

// NewAdminInfoCommand creates `system info` command
func NewAdminInfoCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	i := &infoCmd{out: ioStreams.Out}

	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show vela client and cluster chartPath",
		Long:  "Show vela client and cluster chartPath",
		RunE: func(cmd *cobra.Command, args []string) error {
			return i.run(ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	return cmd
}

func (i *infoCmd) run(ioStreams cmdutil.IOStreams) error {
	clusterVersion, err := GetOAMReleaseVersion(types.DefaultKubeVelaNS)
	if err != nil {
		return fmt.Errorf("fail to get cluster chartPath: %w", err)
	}
	ioStreams.Info("Versions:")
	ioStreams.Infof("kubevela: %s \n", clusterVersion)
	// TODO(wonderflow): we should print all helm charts installed by vela, including plugins

	return nil
}

// GetOAMReleaseVersion gets version of vela-core runtime helm release
func GetOAMReleaseVersion(ns string) (string, error) {
	results, err := helm.GetHelmRelease(ns)
	if err != nil {
		return "", err
	}

	for _, result := range results {
		if result.Chart.ChartFullPath() == types.DefaultKubeVelaChartName {
			return result.Chart.AppVersion(), nil
		}
	}
	return "", errors.New("kubevela chart not found in your kubernetes cluster,  refer to 'https://kubevela.io/docs/install' for installation")
}

// PrintTrackVelaRuntimeStatus prints status of installing vela-core runtime
func PrintTrackVelaRuntimeStatus(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams, trackTimeout time.Duration) (bool, error) {
	trackInterval := 5 * time.Second

	ioStreams.Info("\nIt may take 1-2 minutes before KubeVela runtime is ready.")
	start := time.Now()
	spiner := newTrackingSpinnerWithDelay("Waiting KubeVela runtime ready to serve ...", 5*time.Second)
	spiner.Start()
	defer spiner.Stop()

	for {
		timeConsumed := int(time.Since(start).Seconds())
		applySpinnerNewSuffix(spiner, fmt.Sprintf("Waiting KubeVela runtime ready to serve (timeout %d/%d seconds) ...",
			timeConsumed, int(trackTimeout.Seconds())))

		sts, podName, err := getVelaRuntimeStatus(ctx, c)
		if err != nil {
			return false, err
		}
		if sts == Ready {
			ioStreams.Info(fmt.Sprintf("\n%s %s", emojiSucceed, "KubeVela runtime is ready to serve!"))
			return true, nil
		}
		// status except Ready results in re-check until timeout
		if time.Since(start) > trackTimeout {
			ioStreams.Info(fmt.Sprintf("\n%s %s", emojiFail, "KubeVela runtime starts timeout!"))
			if len(podName) != 0 {
				ioStreams.Info(fmt.Sprintf("\n%s %s%s", emojiLightBulb,
					"Please use this command for more detail: ",
					white.Sprintf("kubectl logs -f %s -n vela-system", podName)))
			}
			return false, nil
		}
		time.Sleep(trackInterval)
	}
}

func getVelaRuntimeStatus(ctx context.Context, c client.Client) (VelaRuntimeStatus, string, error) {
	podList := &corev1.PodList{}
	opts := []client.ListOption{
		client.InNamespace(types.DefaultKubeVelaNS),
		client.MatchingLabels{
			"app.kubernetes.io/name":     types.DefaultKubeVelaChartName,
			"app.kubernetes.io/instance": types.DefaultKubeVelaReleaseName,
		},
	}
	if err := c.List(ctx, podList, opts...); err != nil {
		return Error, "", err
	}
	if len(podList.Items) == 0 {
		return NotFound, "", nil
	}
	runtimePod := podList.Items[0]
	podName := runtimePod.GetName()
	if runtimePod.Status.Phase == corev1.PodRunning {
		// since readiness & liveness probes are set for vela container
		// so check each condition is ready
		for _, c := range runtimePod.Status.Conditions {
			if c.Status != corev1.ConditionTrue {
				return Pending, podName, nil
			}
		}
		return Ready, podName, nil
	}
	return Pending, podName, nil
}
