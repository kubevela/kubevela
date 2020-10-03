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
	"os"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	// VelaConfigName is the name of the configMap that contains the vela config
	VelaConfigName = "vela-config"
)

var (
	helmInstallFunc func(ioStreams cmdutil.IOStreams, c types.Chart) error
)

func init() {
	helmInstallFunc = oam.InstallHelmChart
}

// Setup vela dependency
func Install(client client.Client) error {
	log := ctrl.Log.WithName("vela dependency manager")
	// Fetch the vela configuration
	velaConfigNN := k8stypes.NamespacedName{Name: VelaConfigName, Namespace: types.DefaultOAMNS}
	velaConfig := v1.ConfigMap{}
	if err := client.Get(context.TODO(), velaConfigNN, &velaConfig); err != nil {
		return err
	}
	for crd, chart := range velaConfig.Data {
		log.Info("check on dependency", "crd resource", crd)
		if err := client.Get(context.TODO(), k8stypes.NamespacedName{Name: crd}, &crdv1.CustomResourceDefinition{}); err != nil {
			if apierrors.IsNotFound(err) {
				if instErr := installHelmChart(client, []byte(chart), log); instErr != nil {
					return errors.Wrap(instErr, "failed to install helm chart")
				}
			} else {
				return err
			}
		} else {
			log.Info("resources already exists, skip install", "crd", crd)
		}
	}
	return nil
}

func installHelmChart(client client.Client, chart []byte, log logr.Logger) error {
	ioStreams := cmdutil.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	var helmChart types.Chart
	err := json.Unmarshal(chart, &helmChart)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal the helm chart data")
	}
	log.Info("install helm char", "chart name", helmChart.Name)
	// create the namespace
	if helmChart.Namespace != types.DefaultAppNamespace {
		if len(helmChart.Namespace) > 0 && !cmdutil.IsNamespaceExist(client, helmChart.Namespace) {
			if err = cmdutil.NewNamespace(client, helmChart.Namespace); err != nil {
				return errors.Wrap(err, "failed to create the namespace")
			}
		}
	}
	if err = helmInstallFunc(ioStreams, helmChart); err != nil {
		return err
	}
	return nil
}
