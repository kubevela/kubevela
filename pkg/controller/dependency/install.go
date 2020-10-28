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
	"io/ioutil"
	"os/exec"

	v1 "k8s.io/api/core/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
)

const (
	// VelaConfigName is the name of the configMap that contains the vela config
	VelaConfigName = "vela-config"
)

var (
	log = ctrl.Log.WithName("vela dependency manager")
)

// Setup vela dependency
func Install(kubecli client.Client) error {
	return runCmdFromConfig(kubecli, "install.sh")
}

func Uninstall(kubecli client.Client) error {
	return runCmdFromConfig(kubecli, "uninstall.sh")
}

func runCmdFromConfig(kubecli client.Client, filename string) error {
	velaConfig, err := fetchVelaConfig(kubecli)
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	scriptData := velaConfig.Data[filename]
	if err := ioutil.WriteFile(filename, []byte(scriptData), 0700); err != nil {
		return err
	}
	cmd := exec.Command("/bin/sh", filename)
	out, err := cmd.CombinedOutput()
	log.Info(string(out))
	if err != nil {
		return err
	}
	return nil
}

func fetchVelaConfig(kubecli client.Client) (*v1.ConfigMap, error) {
	velaConfigNN := k8stypes.NamespacedName{Name: VelaConfigName, Namespace: types.DefaultOAMNS}
	velaConfig := &v1.ConfigMap{}
	if err := kubecli.Get(context.TODO(), velaConfigNN, velaConfig); err != nil {
		return nil, err
	}
	return velaConfig, nil
}
