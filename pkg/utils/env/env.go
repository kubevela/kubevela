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

package env

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

const (
	// IndicatingLabel is label key indicating application is an env
	IndicatingLabel = "cli.env.oam.dev/name"
	// RawType is component type of raw
	RawType = "raw"
	// DefaultEnvNamespace is namespace of default env
	DefaultEnvNamespace = "default"
)

// following functions are CRUD of env

// CreateEnv will create e env.
// Because Env equals to namespace, one env should not be updated
func CreateEnv(envArgs *types.EnvMeta) error {
	c, err := common.GetClient()
	if err != nil {
		return err
	}
	if envArgs.Namespace == "" {
		err = common.AskToChooseOneNamespace(c, envArgs)
		if err != nil {
			return err
		}
	}
	if envArgs.Name == "" {
		prompt := &survey.Input{
			Message: "Please name your new env:",
		}
		err = survey.AskOne(prompt, &envArgs.Name)
		if err != nil {
			return err
		}
	}
	ctx := context.TODO()

	var nsList v1.NamespaceList
	err = c.List(ctx, &nsList, client.MatchingLabels{oam.LabelControlPlaneNamespaceUsage: oam.VelaNamespaceUsageEnv,
		oam.LabelNamespaceOfEnvName: envArgs.Name})
	if err != nil {
		return err
	}
	if len(nsList.Items) > 0 {
		return fmt.Errorf("env name %s already exists", envArgs.Name)
	}

	namespace, err := utils.GetNamespace(ctx, c, envArgs.Namespace)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if namespace != nil {
		existedEnv := namespace.GetLabels()[oam.LabelNamespaceOfEnvName]
		if existedEnv != "" && existedEnv != envArgs.Name {
			return fmt.Errorf("the namespace %s was already assigned to env %s", envArgs.Namespace, existedEnv)
		}
	}
	err = utils.CreateOrUpdateNamespace(ctx, c, envArgs.Namespace, utils.MergeOverrideLabels(map[string]string{
		oam.LabelControlPlaneNamespaceUsage: oam.VelaNamespaceUsageEnv,
	}), utils.MergeNoConflictLabels(map[string]string{
		oam.LabelNamespaceOfEnvName: envArgs.Name,
	}))
	if err != nil {
		return err
	}
	return nil

}

// GetEnvByName will get env info by name
func GetEnvByName(name string) (*types.EnvMeta, error) {
	if name == DefaultEnvNamespace {
		return &types.EnvMeta{Name: DefaultEnvNamespace, Namespace: DefaultEnvNamespace}, nil
	}
	namespace, err := getEnvNamespaceByName(name)
	if err != nil {
		return nil, err
	}
	return &types.EnvMeta{
		Name:      name,
		Namespace: namespace.Name,
	}, nil
}

// ListEnvs will list all envs
// if envName specified, return list that only contains one env
func ListEnvs(envName string) ([]*types.EnvMeta, error) {
	var envList []*types.EnvMeta
	if envName != "" {
		env, err := GetEnvByName(envName)
		if err != nil {
			if os.IsNotExist(err) {
				err = fmt.Errorf("env %s not exist", envName)
			}
			return envList, err
		}
		envList = append(envList, env)
		return envList, err
	}
	clt, err := common.GetClient()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	var nsList v1.NamespaceList
	err = clt.List(ctx, &nsList, client.MatchingLabels{oam.LabelControlPlaneNamespaceUsage: oam.VelaNamespaceUsageEnv})
	if err != nil {
		return nil, err
	}
	for _, it := range nsList.Items {
		envList = append(envList, &types.EnvMeta{
			Name:      it.Labels[oam.LabelNamespaceOfEnvName],
			Namespace: it.Name,
		})
	}
	if len(envList) < 1 {
		return envList, nil
	}
	cur, err := GetCurrentEnv()
	if err != nil {
		_ = SetCurrentEnv(envList[0])
		envList[0].Current = "*"
		// we set a current env if not exist
		// nolint:nilerr
		return envList, nil
	}
	for i := range envList {
		if envList[i].Name == cur.Name {
			envList[i].Current = "*"
		}
	}
	return envList, nil
}

// DeleteEnv will delete env and its application
func DeleteEnv(envName string) (string, error) {
	var message string
	var err error
	envMeta, err := GetEnvByName(envName)
	if err != nil {
		return "", err
	}
	clt, err := common.GetClient()
	if err != nil {
		return "", err
	}
	var appList v1beta1.ApplicationList
	err = clt.List(context.TODO(), &appList, client.InNamespace(envMeta.Namespace))
	if err != nil {
		return "", err
	}
	if len(appList.Items) > 0 {
		err = fmt.Errorf("you can't delete this environment(namespace=%s) that has %d application inside", envMeta.Namespace, len(appList.Items))
		return message, err
	}
	// reset the labels
	err = utils.UpdateNamespace(context.TODO(), clt, envMeta.Namespace, utils.MergeOverrideLabels(map[string]string{
		oam.LabelNamespaceOfEnvName:         "",
		oam.LabelControlPlaneNamespaceUsage: "",
	}))
	if err != nil {
		return "", err
	}
	message = "env " + envName + " deleted"
	return message, err
}

// GetCurrentEnv will get current env
func GetCurrentEnv() (*types.EnvMeta, error) {
	currentEnvPath, err := system.GetCurrentEnvPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Clean(currentEnvPath))
	if err != nil {
		return nil, err
	}
	var envMeta types.EnvMeta
	err = json.Unmarshal(data, &envMeta)
	if err != nil {
		em, err := GetEnvByName(string(data))
		if err != nil {
			return nil, err
		}
		_ = SetCurrentEnv(em)
		return em, nil
	}
	return &envMeta, nil
}

// SetCurrentEnv will set the current env to the specified one
func SetCurrentEnv(meta *types.EnvMeta) error {
	currentEnvPath, err := system.GetCurrentEnvPath()
	if err != nil {
		return err
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	//nolint:gosec
	if err = os.WriteFile(currentEnvPath, data, 0644); err != nil {
		return err
	}
	return nil
}

// getEnvNamespaceByName get v1.Namespace object by env name
func getEnvNamespaceByName(name string) (*v1.Namespace, error) {
	clt, err := common.GetClient()
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	var nsList v1.NamespaceList
	err = clt.List(ctx, &nsList, client.MatchingLabels{oam.LabelNamespaceOfEnvName: name})
	if err != nil {
		return nil, err
	}
	if len(nsList.Items) < 1 {
		return nil, errors.Errorf("Env %s not exist", name)
	}

	return &nsList.Items[0], nil
}

// SetEnvLabels set labels for namespace
func SetEnvLabels(envArgs *types.EnvMeta) error {
	c, err := common.GetClient()
	if err != nil {
		return err
	}

	namespace, err := getEnvNamespaceByName(envArgs.Name)
	if err != nil {
		return err
	}
	labels, err := labels.ConvertSelectorToLabelsMap(envArgs.Labels)
	if err != nil {
		return err
	}

	namespace.Labels = util.MergeMapOverrideWithDst(namespace.GetLabels(), labels)

	err = c.Update(context.Background(), namespace)
	if err != nil {
		return errors.Wrapf(err, "fail to set env labels")
	}
	return nil
}
