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
	"time"

	"cuelang.org/go/pkg/strings"
	"github.com/hashicorp/go-version"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/strvals"
	corev1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevela/pkg/util/k8s"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	innerVersion "github.com/oam-dev/kubevela/version"
)

// defaultConstraint
const defaultConstraint = ">= 1.19, <= 1.24"

const kubevelaInstallerHelmRepoURL = "https://charts.kubevela.net/core/"

// kubeVelaReleaseName release name
const kubeVelaReleaseName = "kubevela"

// kubeVelaChartName the name of veal core chart
const kubeVelaChartName = "vela-core"

// InstallArgs the args for install command
type InstallArgs struct {
	userInput     *UserInput
	helmHelper    *helm.Helper
	Args          common.Args
	Values        []string
	Namespace     string
	Version       string
	ChartFilePath string
	Detail        bool
	ReuseValues   bool
}

// NewInstallCommand creates `install` command to install vela core
func NewInstallCommand(c common.Args, order string, ioStreams util.IOStreams) *cobra.Command {
	installArgs := &InstallArgs{Args: c, userInput: NewUserInput(), helmHelper: helm.NewHelper()}
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Installs or Upgrades Kubevela control plane on a Kubernetes cluster.",
		Long:  "The Kubevela CLI allows installing Kubevela on any Kubernetes derivative to which your kube config is pointing to.",
		Args:  cobra.ExactArgs(0),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// CheckRequirements
			ioStreams.Info("Check Requirements ...")
			restConfig, err := c.GetConfig()
			if err != nil {
				return errors.Wrapf(err, "failed to get kube config, You can set KUBECONFIG env or make file ~/.kube/config")
			}
			if isNewerVersion, serverVersion, err := checkKubeServerVersion(restConfig); err != nil {
				ioStreams.Error(err.Error())
				ioStreams.Error("This is not recommended and could have negative impacts on the stability of KubeVela - use at your own risk.")

				userConfirmation := installArgs.userInput.AskBool("Do you want to continue?", &UserInputOptions{assumeYes})
				if !userConfirmation {
					return fmt.Errorf("stopping installation")
				}
			} else if isNewerVersion {
				ioStreams.Errorf("The Kubernetes server version(%s) is higher than the one officially supported(%s).\n", serverVersion, defaultConstraint)
				ioStreams.Error("This is not recommended and could have negative impacts on the stability of KubeVela - use at your own risk.")
				userInput := NewUserInput()
				userConfirmation := userInput.AskBool("Do you want to continue?", &UserInputOptions{assumeYes})
				if !userConfirmation {
					return fmt.Errorf("stopping installation")
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Step1: Download Helm Chart
			ioStreams.Info("Installing KubeVela Core ...")
			if installArgs.ChartFilePath == "" {
				installArgs.ChartFilePath = getKubeVelaHelmChartRepoURL(installArgs.Version)
			}
			chart, err := installArgs.helmHelper.LoadCharts(installArgs.ChartFilePath, nil)
			if err != nil {
				return fmt.Errorf("loadding the helm chart of kubeVela control plane failure, %w", err)
			}
			ioStreams.Infof("Helm Chart used for KubeVela control plane installation: %s \n", installArgs.ChartFilePath)

			// Step2: Prepare namespace
			restConfig, err := c.GetConfig()
			if err != nil {
				return fmt.Errorf("get kube config failure: %w", err)
			}
			kubeClient, err := c.GetClient()
			if err != nil {
				return fmt.Errorf("create kube client failure: %w", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()
			var namespace corev1.Namespace
			var namespaceExists = true
			if err := kubeClient.Get(ctx, apitypes.NamespacedName{Name: installArgs.Namespace}, &namespace); err != nil {
				if !apierror.IsNotFound(err) {
					return fmt.Errorf("failed to check if namespace %s already exists: %w", installArgs.Namespace, err)
				}
				namespaceExists = false
			}
			if namespaceExists {
				fmt.Printf("Existing KubeVela installation found in namespace %s\n\n", installArgs.Namespace)
				userConfirmation := installArgs.userInput.AskBool("Do you want to overwrite this installation?", &UserInputOptions{assumeYes})
				if !userConfirmation {
					return fmt.Errorf("stopping installation")
				}
			} else {
				namespace.Name = installArgs.Namespace
				if err := kubeClient.Create(ctx, &namespace); err != nil {
					return fmt.Errorf("failed to create kubeVela namespace %s: %w", installArgs.Namespace, err)
				}
			}

			if err := checkExistStepDefinitions(ctx, kubeClient, namespace.Name); err != nil {
				return err
			}
			if err := checkExistViews(ctx, kubeClient, namespace.Name); err != nil {
				return err
			}

			// Step3: Prepare the values for chart
			imageTag := installArgs.Version
			if !strings.HasPrefix(imageTag, "v") {
				imageTag = "v" + imageTag
			}
			var values = map[string]interface{}{
				"image": map[string]interface{}{
					"tag":        imageTag,
					"pullPolicy": "IfNotPresent",
				},
			}
			if len(installArgs.Values) > 0 {
				for _, value := range installArgs.Values {
					if err := strvals.ParseInto(value, values); err != nil {
						return errors.Wrap(err, "failed parsing --set data")
					}
				}
			}
			// Step4: apply new CRDs
			if err := upgradeCRDs(cmd.Context(), kubeClient, chart); err != nil {
				return fmt.Errorf("upgrade CRD failure %w", err)
			}
			// Step5: Install or upgrade helm release
			release, err := installArgs.helmHelper.UpgradeChart(chart, kubeVelaReleaseName, installArgs.Namespace, values,
				helm.UpgradeChartOptions{
					Config:      restConfig,
					Detail:      installArgs.Detail,
					Logging:     ioStreams,
					Wait:        true,
					ReuseValues: installArgs.ReuseValues,
				})
			if err != nil {
				msg := fmt.Sprintf("Could not install KubeVela control plane installation: %s", err.Error())
				return errors.New(msg)
			}

			err = waitKubeVelaControllerRunning(kubeClient, installArgs.Namespace, release.Manifest)
			if err != nil {
				msg := fmt.Sprintf("Could not complete KubeVela control plane installation: %s \nFor troubleshooting, please check the status of the kubevela deployment by executing the following command: \n\nkubectl get pods -n %s\n", err.Error(), installArgs.Namespace)
				return errors.New(msg)
			}
			ioStreams.Info()
			ioStreams.Info("KubeVela control plane has been successfully set up on your cluster.")
			ioStreams.Info("If you want to enable dashboard, please run \"vela addon enable velaux\"")
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeSystem,
		},
	}

	cmd.Flags().StringArrayVarP(&installArgs.Values, "set", "", []string{}, "set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")
	cmd.Flags().StringVarP(&installArgs.Namespace, "namespace", "n", "vela-system", "namespace scope for installing KubeVela Core")
	cmd.Flags().StringVarP(&installArgs.Version, "version", "v", innerVersion.VelaVersion, "")
	cmd.Flags().BoolVarP(&installArgs.Detail, "detail", "d", true, "show detail log of installation")
	cmd.Flags().BoolVarP(&installArgs.ReuseValues, "reuse", "r", true, "will re-use the user's last supplied values.")
	cmd.Flags().StringVarP(&installArgs.ChartFilePath, "file", "f", "", "custom the chart path of KubeVela control plane")
	return cmd
}

func checkKubeServerVersion(config *rest.Config) (bool, string, error) {
	// get kubernetes cluster api version
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return false, "", err
	}
	// check version
	serverVersion, err := client.ServerVersion()
	if err != nil {
		return false, "", fmt.Errorf("get kubernetes api version failure %w", err)
	}
	vStr := fmt.Sprintf("%s.%s", serverVersion.Major, strings.Replace(serverVersion.Minor, "+", "", 1))
	currentVersion, err := version.NewVersion(vStr)
	if err != nil {
		return false, "", err
	}
	hConstraints, err := version.NewConstraint(defaultConstraint)
	if err != nil {
		return false, "", err
	}
	isNewerVersion, allConstraintsValid := checkIsNewVersion(hConstraints, currentVersion)

	if allConstraintsValid {
		return false, vStr, nil
	}
	if isNewerVersion {
		return true, vStr, nil
	}

	return false, vStr, fmt.Errorf("the kubernetes server version '%s' doesn't satisfy constraints '%s'", serverVersion, defaultConstraint)
}

// checkIsNewVersion checks if the provided version is higher than all constraints and if all constraints are valid
func checkIsNewVersion(hConstraints version.Constraints, serverVersion *version.Version) (bool, bool) {
	isNewerVersion := false
	allConstraintsValid := true
	for _, constraint := range hConstraints {
		validConstraint := constraint.Check(serverVersion)
		if !validConstraint {
			allConstraintsValid = false
			constraintVersionString := getConstraintVersion(constraint.String())
			constraintVersion, err := version.NewVersion(constraintVersionString)
			if err != nil {
				return false, false
			}
			if serverVersion.GreaterThan(constraintVersion) {
				isNewerVersion = true
			} else {
				return false, false
			}
		}
	}
	return isNewerVersion, allConstraintsValid
}

// getConstraintVersion returns the version of a constraint without leading spaces, <, >, =
func getConstraintVersion(constraint string) string {
	for index, character := range constraint {
		if character != '<' && character != '>' && character != ' ' && character != '=' {
			return constraint[index:]
		}
	}
	return constraint
}

func getKubeVelaHelmChartRepoURL(version string) string {
	// Determine installer version
	if innerVersion.IsOfficialKubeVelaVersion(version) {
		version, _ := innerVersion.GetOfficialKubeVelaVersion(version)
		return kubevelaInstallerHelmRepoURL + kubeVelaChartName + "-" + version + ".tgz"
	}
	return kubevelaInstallerHelmRepoURL + kubeVelaChartName + "-" + version + ".tgz"
}

func waitKubeVelaControllerRunning(kubeClient client.Client, namespace, manifest string) error {
	deployments := helm.GetDeploymentsFromManifest(manifest)
	spinner := newTrackingSpinnerWithDelay("Waiting KubeVela control plane running ...", 1*time.Second)
	spinner.Start()
	defer spinner.Stop()
	trackInterval := 5 * time.Second
	timeout := 600 * time.Second
	start := time.Now()
	ctx := context.Background()
	for {
		timeConsumed := int(time.Since(start).Seconds())
		var readyCount = 0
		for i, d := range deployments {
			err := kubeClient.Get(ctx, apitypes.NamespacedName{Name: d.Name, Namespace: namespace}, deployments[i])
			if err != nil {
				return client.IgnoreNotFound(err)
			}
			if deployments[i].Status.ReadyReplicas != deployments[i].Status.Replicas {
				applySpinnerNewSuffix(spinner, fmt.Sprintf("Waiting deployment %s ready. (timeout %d/%d seconds)...", deployments[i].Name, timeConsumed, int(timeout.Seconds())))
			} else {
				readyCount++
			}
		}
		if readyCount >= len(deployments) {
			return nil
		}
		if timeConsumed > int(timeout.Seconds()) {
			return errors.Errorf("Enabling timeout, please run \"kubectl get pod -n vela-system\" to check the status")
		}
		time.Sleep(trackInterval)
	}
}

func upgradeCRDs(ctx context.Context, kubeClient client.Client, chart *chart.Chart) error {
	crds := helm.GetCRDFromChart(chart)
	applyHelper := apply.NewAPIApplicator(kubeClient)
	for _, crd := range crds {
		if err := applyHelper.Apply(ctx, crd, apply.DisableUpdateAnnotation()); err != nil {
			return err
		}
	}
	return nil
}

func checkExistStepDefinitions(ctx context.Context, kubeClient client.Client, namespace string) error {
	legacyDefs := []string{"apply-deployment", "apply-terraform-config", "apply-terraform-provider", "clean-jobs", "request", "vela-cli"}
	for _, name := range legacyDefs {
		def := &v1beta1.WorkflowStepDefinition{}
		if err := kubeClient.Get(ctx, apitypes.NamespacedName{Namespace: namespace, Name: name}, def); err == nil {
			if err := takeOverResourcesForHelm(ctx, kubeClient, def, namespace); err != nil {
				return fmt.Errorf("failed to update the %s workflow step definition: %w", name, err)
			}
			klog.Infof("successfully tack over the %s workflow step definition", name)
		}
	}
	return nil
}

func checkExistViews(ctx context.Context, kubeClient client.Client, namespace string) error {
	legacyViews := []string{"component-pod-view", "component-service-view"}
	for _, name := range legacyViews {
		cm := &corev1.ConfigMap{}
		if err := kubeClient.Get(ctx, apitypes.NamespacedName{Namespace: namespace, Name: name}, cm); err == nil {
			if err := takeOverResourcesForHelm(ctx, kubeClient, cm, namespace); err != nil {
				return fmt.Errorf("failed to update the %s view: %w", name, err)
			}
			klog.Infof("successfully tack over the %s view", name)
		}
	}
	return nil
}

func takeOverResourcesForHelm(ctx context.Context, kubeClient client.Client, obj client.Object, namespace string) error {
	anno := obj.GetAnnotations()
	if anno != nil && anno["meta.helm.sh/release-name"] == kubeVelaReleaseName {
		return nil
	}
	if err := k8s.AddLabel(obj, "app.kubernetes.io/managed-by", "Helm"); err != nil {
		return err
	}
	if err := k8s.AddAnnotation(obj, "meta.helm.sh/release-name", kubeVelaReleaseName); err != nil {
		return err
	}
	if err := k8s.AddAnnotation(obj, "meta.helm.sh/release-namespace", namespace); err != nil {
		return err
	}
	return kubeClient.Update(ctx, obj)
}
