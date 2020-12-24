/*
Copyright 2019 The Crossplane Authors.

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

package dependency

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
)

const (
	// VelaConfigName is the name of the configMap that contains the vela config
	VelaConfigName = "vela-config"
)

var (
	log             = ctrl.Log.WithName("dependency installer")
	helmInstallFunc func(ioStreams cmdutil.IOStreams, c types.Chart) error
)

func init() {
	helmInstallFunc = helm.InstallHelmChart
}

// Install will setup vela dependency.
// Failing to install vela dependency should not block server from starting up.
// Some users might fail to get charts due to network blockage. We should fix this in other ways.
// Note: reconsider delegating the work to helm operator.
func Install(kubecli client.Client) {
	velaConfig, err := fetchVelaConfig(kubecli)
	if apierrors.IsNotFound(err) {
		log.Info("no ConfigMap('vela-config') found in vela-system namespace, will not install any dependency")
		return
	}
	if err != nil {
		log.Error(err, "fetch ConfigMap('vela-config') in vela-system namespace failed")
		return
	}
	for key, chart := range velaConfig.Data {
		err := installHelmChart([]byte(chart), log)
		if err != nil {
			log.Error(err, fmt.Sprintf("failed to install helm chart for %s", key))
		}
		if key == "servicemonitors.monitoring.coreos.com" {
			if err = InstallPrometheusInstance(kubecli); err != nil {
				log.Error(err, "failed to install prometheus Instance")
			}
		}
	}
}

// Uninstall will uninstall kubevela helm release
func Uninstall(kubecli client.Client) {
	velaConfig, err := fetchVelaConfig(kubecli)
	if err != nil {
		log.Error(err, "fetchVelaConfig failed")
	}
	if velaConfig == nil {
		return
	}
	for _, chart := range velaConfig.Data {
		err := uninstallHelmChart([]byte(chart), log)
		if err != nil {
			log.Error(err, "failed to uninstall helm chart")
		}
	}
}

func fetchVelaConfig(kubecli client.Client) (*v1.ConfigMap, error) {
	velaConfigNN := k8stypes.NamespacedName{Name: VelaConfigName, Namespace: types.DefaultKubeVelaNS}
	velaConfig := &v1.ConfigMap{}
	if err := kubecli.Get(context.TODO(), velaConfigNN, velaConfig); err != nil {
		return nil, err
	}
	return velaConfig, nil
}

func installHelmChart(chart []byte, log logr.Logger) error {
	ioStreams := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	var helmChart types.Chart
	err := json.Unmarshal(chart, &helmChart)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal the helm chart data")
	}
	log.Info("installing helm chart", "chart name", helmChart.Name)
	if err = helmInstallFunc(ioStreams, helmChart); err != nil {
		return err
	}
	return nil
}

func uninstallHelmChart(chart []byte, log logr.Logger) error {
	io := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	var c types.Chart
	err := json.Unmarshal(chart, &c)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal the helm chart data")
	}
	log.Info("uninstalling helm chart", "chart name", c.Name)
	if err = helm.Uninstall(io, c.Name, c.Namespace, c.Name); err != nil {
		return err
	}
	return nil
}
