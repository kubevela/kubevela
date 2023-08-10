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

package common

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// InstallComponentDefinition will add a component into K8s cluster and install its controller
func InstallComponentDefinition(client client.Client, componentData []byte, ioStreams cmdutil.IOStreams) error {
	var cd v1beta1.ComponentDefinition
	var err error
	if componentData == nil {
		return errors.New("componentData is nil")
	}
	if err = yaml.Unmarshal(componentData, &cd); err != nil {
		return err
	}
	cd.Namespace = types.DefaultKubeVelaNS
	ioStreams.Info("Installing component: " + cd.Name)
	if err = client.Create(context.Background(), &cd); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// InstallTraitDefinition will add a trait into K8s cluster and install it's controller
func InstallTraitDefinition(client client.Client, traitdata []byte, ioStreams cmdutil.IOStreams) error {
	var td v1beta1.TraitDefinition
	var err error
	if err = yaml.Unmarshal(traitdata, &td); err != nil {
		return err
	}
	td.Namespace = types.DefaultKubeVelaNS
	ioStreams.Info("Installing trait " + td.Name)
	if err = client.Create(context.Background(), &td); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func addSourceIntoExtension(in *runtime.RawExtension, source *types.Source) error {
	var extension map[string]interface{}
	err := json.Unmarshal(in.Raw, &extension)
	if err != nil {
		return err
	}
	extension["source"] = source
	data, err := json.Marshal(extension)
	if err != nil {
		return err
	}
	in.Raw = data
	return nil
}

// CheckLabelExistence checks whether a label `key=value` exists in definition labels
func CheckLabelExistence(labels map[string]string, label string) bool {
	splitLabel := strings.Split(label, "=")
	if len(splitLabel) < 2 {
		return false
	}
	k, v := splitLabel[0], splitLabel[1]
	if labelValue, ok := labels[k]; ok {
		if labelValue == v {
			return true
		}
	}
	return false
}
