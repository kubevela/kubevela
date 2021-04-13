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
	"io/ioutil"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/strvals"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/hack/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/plugins"
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

type initCmd struct {
	namespace string
	ioStreams cmdutil.IOStreams
	client    client.Client
	chartPath string
	chartArgs chartArgs
	waitReady string
	c         common.Args
}

type chartArgs struct {
	imageRepo       string
	imageTag        string
	imagePullPolicy string
	more            []string
}

type infoCmd struct {
	out io.Writer
}

// SystemCommandGroup creates `system` command and its nested children command
func SystemCommandGroup(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "System management utilities",
		Long:  "System management utilities",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.AddCommand(NewLiveDiffCommand(c, ioStream))
	cmd.AddCommand(NewDryRunCommand(c, ioStream))
	cmd.AddCommand(NewAdminInfoCommand(ioStream))
	cmd.AddCommand(NewCUEPackageCommand(c, ioStream))
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

// NewInstallCommand creates `install` command
func NewInstallCommand(c common.Args, chartContent string, ioStreams cmdutil.IOStreams) *cobra.Command {
	i := &initCmd{ioStreams: ioStreams}
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Vela Core with built-in capabilities",
		Long:  "Install Vela Core with built-in capabilities",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}
			i.client = newClient
			i.namespace = types.DefaultKubeVelaNS
			i.c = c
			return i.run(ioStreams, chartContent)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeStart,
		},
		Deprecated: "vela install is DEPRECATED and we will remove it after Kubevela 1.0. Please use helm chart instead",
	}

	flag := cmd.Flags()
	flag.StringVarP(&i.chartPath, "vela-chart-path", "p", "", "path to vela core chart to override default chart")
	flag.StringVarP(&i.chartArgs.imagePullPolicy, "image-pull-policy", "", "", "vela core image pull policy, this will align to chart value image.pullPolicy")
	flag.StringVarP(&i.chartArgs.imageRepo, "image-repo", "", "", "vela core image repo, this will align to chart value image.repo")
	flag.StringVarP(&i.chartArgs.imageTag, "image-tag", "", "", "vela core image repo, this will align to chart value image.tag")
	flag.StringVarP(&i.waitReady, "wait", "w", "0s", "wait until vela-core is ready to serve, default will not wait")
	flag.StringSliceVarP(&i.chartArgs.more, "set", "s", []string{}, "arguments for installing vela-core chart")

	return cmd
}

func (i *initCmd) run(ioStreams cmdutil.IOStreams, chartSource string) error {
	waitDuration, err := time.ParseDuration(i.waitReady)
	if err != nil {
		return fmt.Errorf("invalid wait timeoout duration %w, should use '120s', '5m' like format", err)
	}

	ioStreams.Info("- Installing Vela Core Chart:")
	exist, err := cmdutil.DoesNamespaceExist(i.client, types.DefaultKubeVelaNS)
	if err != nil {
		return err
	}
	if !exist {
		if err := cmdutil.NewNamespace(i.client, types.DefaultKubeVelaNS); err != nil {
			return err
		}
		ioStreams.Info("created namespace", types.DefaultKubeVelaNS)
	}

	if helm.IsHelmReleaseRunning(types.DefaultKubeVelaReleaseName, types.DefaultKubeVelaChartName, types.DefaultKubeVelaNS, i.ioStreams) {
		i.ioStreams.Info("Vela system along with OAM runtime already exist.")
	} else {
		vals, err := i.resolveValues()
		if err != nil {
			i.ioStreams.Errorf("resolve values for vela-core chart err %v, will install with default values", err)
			vals = make(map[string]interface{})
		}
		if err := InstallOamRuntime(i.chartPath, chartSource, vals, ioStreams); err != nil {
			return err
		}
	}
	if err = CheckCapabilityReady(context.Background(), i.c, waitDuration); err != nil {
		ioStreams.Infof("- Vela-Core was installed successfully while some capabilities were still installing background, "+
			"try running 'vela workloads' or 'vela traits' to check after a while, details: %v", err)
		return nil
	}
	ioStreams.Info("- Finished successfully.")

	if waitDuration > 0 {
		_, err := PrintTrackVelaRuntimeStatus(context.Background(), i.client, ioStreams, waitDuration)
		if err != nil {
			return err
		}
	}
	return nil
}

// CheckCapabilityReady waits unitl capability is installed successfully
func CheckCapabilityReady(ctx context.Context, c common.Args, timeout time.Duration) error {
	if timeout < 5*time.Minute {
		timeout = 5 * time.Minute
	}
	tmpdir, err := ioutil.TempDir(".", "tmpcap")
	if err != nil {
		return err
	}
	//nolint:errcheck
	defer os.RemoveAll(tmpdir)

	start := time.Now()
	spiner := newTrackingSpinnerWithDelay("Waiting Capability ready to install ...", 10*time.Second)
	spiner.Start()
	defer spiner.Stop()

	for {
		_, err = plugins.GetCapabilitiesFromCluster(ctx, types.DefaultKubeVelaNS, c, nil)
		if err == nil {
			return nil
		}
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout when checking capability readiness: \nWarning: %w", err)
		}
		time.Sleep(5 * time.Second)
	}
}

func (i *initCmd) resolveValues() (map[string]interface{}, error) {
	finalValues := map[string]interface{}{}
	var valuesConfig []string
	// By default align values.yaml in chart
	if i.chartArgs.imageRepo != "" {
		valuesConfig = append(valuesConfig, fmt.Sprintf("image.repository=%s", i.chartArgs.imageRepo))
	}
	if i.chartArgs.imageTag != "" {
		valuesConfig = append(valuesConfig, fmt.Sprintf("image.tag=%s", i.chartArgs.imageTag))
	}
	if i.chartArgs.imagePullPolicy != "" {
		valuesConfig = append(valuesConfig, fmt.Sprintf("image.pullPolicy=%s", i.chartArgs.imagePullPolicy))
	}
	valuesConfig = append(valuesConfig, i.chartArgs.more...)

	for _, val := range valuesConfig {
		// parses Helm strvals line and merges into a map for the final overrides for values.yaml
		if err := strvals.ParseInto(val, finalValues); err != nil {
			return nil, err
		}
	}
	return finalValues, nil
}

// InstallOamRuntime installs vela-core runtime from helm chart
func InstallOamRuntime(chartPath, chartSource string, vals map[string]interface{}, ioStreams cmdutil.IOStreams) error {
	var err error
	var chartRequested *chart.Chart
	if chartPath != "" {
		ioStreams.Infof("Use customized chart at: %s", chartPath)
		chartRequested, err = loader.Load(chartPath)
	} else {
		chartRequested, err = utils.LoadChart(chartSource)
		if chartRequested != nil {
			m, l := chartRequested.Metadata, len(chartRequested.Raw)
			ioStreams.Infof("install chart %s, version %s, desc : %s, contains %d file\n", m.Name, m.Version, m.Description, l)
		}
	}
	if err != nil {
		return fmt.Errorf("error loading chart for installation: %w", err)
	}
	installClient, err := helm.NewHelmInstall("", types.DefaultKubeVelaNS, types.DefaultKubeVelaReleaseName)
	if err != nil {
		return fmt.Errorf("error create helm install client: %w", err)
	}
	release, err := installClient.Run(chartRequested, vals)
	if err != nil {
		ioStreams.Errorf("Failed to install the chart with error: %+v\n", err)
		return err
	}
	ioStreams.Infof("Successfully installed the chart, status: %s, last deployed time = %s\n",
		release.Info.Status,
		release.Info.LastDeployed.String())
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
