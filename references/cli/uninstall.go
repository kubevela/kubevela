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
	"bufio"
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

// UnInstallArgs the args for uninstall command
type UnInstallArgs struct {
	userInput  *UserInput
	helmHelper *helm.Helper
	Args       common.Args
	Namespace  string
	Detail     bool
	force      bool
	cancel     bool
}

// NewUnInstallCommand creates `uninstall` command to uninstall vela core
func NewUnInstallCommand(c common.Args, order string, ioStreams util.IOStreams) *cobra.Command {
	unInstallArgs := &UnInstallArgs{Args: c, userInput: &UserInput{
		Writer: ioStreams.Out,
		Reader: bufio.NewReader(ioStreams.In),
	}, helmHelper: helm.NewHelper()}
	cmd := &cobra.Command{
		Use:     "uninstall",
		Short:   "Uninstalls KubeVela from a Kubernetes cluster",
		Example: `vela uninstall`,
		Long:    "Uninstalls KubeVela from a Kubernetes cluster.",
		Args:    cobra.ExactArgs(0),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			unInstallArgs.cancel = unInstallArgs.userInput.AskBool("Would you like to uninstall KubeVela from this cluster?", &UserInputOptions{AssumeYes: assumeYes})
			if !unInstallArgs.cancel {
				return nil
			}
			kubeClient, err := c.GetClient()
			if err != nil {
				return errors.Wrapf(err, "failed to get kube client")
			}

			if !unInstallArgs.force {
				// if use --force flag will skip checking the addon
				addons, err := checkInstallAddon(kubeClient)
				if err != nil {
					return errors.Wrapf(err, "cannot check installed addon")
				}
				if len(addons) != 0 {
					return fmt.Errorf("these addons have been enabled :%v, please guarantee there is no application using these addons and use `vela uninstall -f` uninstall include addon ", addons)
				}
			}

			// filter out addon related app, these app will be delete by force uninstall
			// ignore the error, this error cannot be not nil
			labels, _ := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{Key: oam.LabelAddonName, Operator: metav1.LabelSelectorOpDoesNotExist}}})
			var apps v1beta1.ApplicationList
			err = kubeClient.List(context.Background(), &apps, &client.ListOptions{
				Namespace:     "",
				LabelSelector: labels,
			})
			if err != nil {
				return errors.Wrapf(err, "failed to check app in cluster")
			}
			if len(apps.Items) > 0 {
				return fmt.Errorf("please delete all applications before uninstall. using \"vela ls -A\" view all applications")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !unInstallArgs.cancel {
				return nil
			}
			ioStreams.Info("Starting to uninstall KubeVela")
			restConfig, err := c.GetConfig()
			if err != nil {
				return errors.Wrapf(err, "failed to get kube config, You can set KUBECONFIG env or make file ~/.kube/config")
			}
			kubeClient, err := c.GetClient()
			if err != nil {
				return errors.Wrapf(err, "failed to get kube client")
			}
			if unInstallArgs.force {
				// if use --force disable all addons
				err := forceDisableAddon(context.Background(), kubeClient, restConfig)
				if err != nil {
					return errors.Wrapf(err, "cannot force disabe all addons")
				}
			}
			if err := unInstallArgs.helmHelper.UninstallRelease(kubeVelaReleaseName, unInstallArgs.Namespace, restConfig, unInstallArgs.Detail, ioStreams); err != nil {
				return err
			}
			// Clean up vela-system namespace
			if err := deleteNamespace(kubeClient, unInstallArgs.Namespace); err != nil {
				return err
			}
			var namespace corev1.Namespace
			var namespaceExists = true
			if err := kubeClient.Get(cmd.Context(), apitypes.NamespacedName{Name: "kubevela"}, &namespace); err != nil {
				if !apierror.IsNotFound(err) {
					return fmt.Errorf("failed to check if namespace kubevela already exists: %w", err)
				}
				namespaceExists = false
			}
			if namespaceExists {
				fmt.Printf("The namespace kubevela is exist, it is the default database of the velaux\n\n")
				userConfirmation := unInstallArgs.userInput.AskBool("Do you want to delete it?", &UserInputOptions{assumeYes})
				if userConfirmation {
					if err := deleteNamespace(kubeClient, "kubevela"); err != nil {
						return err
					}
				}
			}
			ioStreams.Info("Successfully uninstalled KubeVela")
			ioStreams.Info("Please delete all CRD from cluster using \"kubectl get crd |grep oam | awk '{print $1}' | xargs kubectl delete crd\"")
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeSystem,
		},
	}

	cmd.Flags().StringVarP(&unInstallArgs.Namespace, "namespace", "n", "vela-system", "namespace scope for installing KubeVela Core")
	cmd.Flags().BoolVarP(&unInstallArgs.Detail, "detail", "d", true, "show detail log of installation")
	cmd.Flags().BoolVarP(&unInstallArgs.force, "force", "f", false, "force uninstall whole vela include all addons")
	return cmd
}

func deleteNamespace(kubeClient client.Client, namespace string) error {
	var ns corev1.Namespace
	ns.Name = namespace
	return kubeClient.Delete(context.Background(), &ns)
}

func checkInstallAddon(kubeClient client.Client) ([]string, error) {
	apps := &v1beta1.ApplicationList{}
	if err := kubeClient.List(context.Background(), apps, client.InNamespace(types.DefaultKubeVelaNS), client.HasLabels{oam.LabelAddonName}); err != nil {
		return nil, err
	}
	var res []string
	for _, application := range apps.Items {
		res = append(res, application.Labels[oam.LabelAddonName])
	}
	return res, nil
}

// forceDisableAddon force delete all enabled addons, fluxcd must be the last one to be deleted
func forceDisableAddon(ctx context.Context, kubeClient client.Client, config *rest.Config) error {
	addons, err := checkInstallAddon(kubeClient)
	if err != nil {
		return errors.Wrapf(err, "cannot check the installed addon")
	}
	// fluxcd addon should be deleted lastly
	fluxcdFlag := false
	for _, addon := range addons {
		if addon == "fluxcd" {
			fluxcdFlag = true
			continue
		}
		if err := pkgaddon.DisableAddon(ctx, kubeClient, addon, config, true); err != nil {
			return err
		}
	}
	if fluxcdFlag {
		timeConsumed := time.Now()
		var addons []string
		for {
			// block 5 minute until other addons have been deleted
			if time.Now().After(timeConsumed.Add(5 * time.Minute)) {
				return fmt.Errorf("timeout disable addon, please disable theis addons: %v", addons)
			}
			addons, err = checkInstallAddon(kubeClient)
			if err != nil {
				return err
			}
			if len(addons) == 1 && addons[0] == "fluxcd" {
				break
			}
			time.Sleep(5 * time.Second)
		}
		if err := pkgaddon.DisableAddon(ctx, kubeClient, "fluxcd", config, true); err != nil {
			return err
		}
		timeConsumed = time.Now()
		for {
			if time.Now().After(timeConsumed.Add(5 * time.Minute)) {
				return errors.New("timeout disable fluxcd addon, please disable the addon manually")
			}
			addons, err := checkInstallAddon(kubeClient)
			if err != nil {
				return err
			}
			if len(addons) == 0 {
				break
			}
			fmt.Printf("Waiting delete the fluxcd addon, timeout left %s . \r\n", 5*time.Minute-time.Since(timeConsumed))
			time.Sleep(2 * time.Second)
		}
	}
	return nil
}
