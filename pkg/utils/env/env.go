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
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

// GetEnvDirByName will get env dir from name
func GetEnvDirByName(name string) string {
	envdir, _ := system.GetEnvDir()
	return filepath.Join(envdir, name)
}

// GetEnvByName will get env info by name
func GetEnvByName(name string) (*types.EnvMeta, error) {
	data, err := ioutil.ReadFile(filepath.Join(GetEnvDirByName(name), system.EnvConfigName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("env %s not exist", name)
		}
		return nil, err
	}
	var meta types.EnvMeta
	if err = json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// CreateOrUpdateEnv will create or update env.
// If it does not exist, create it and set to the new env.
// If it exists, update it and set to the new env.
func CreateOrUpdateEnv(ctx context.Context, c client.Client, envName string, envArgs *types.EnvMeta) (string, error) {

	createOrUpdated := "created"
	old, err := GetEnvByName(envName)
	if err == nil {
		createOrUpdated = "updated"
		if envArgs.Domain == "" {
			envArgs.Domain = old.Domain
		}
		if envArgs.Email == "" {
			envArgs.Email = old.Email
		}
		if envArgs.Namespace == "" {
			envArgs.Namespace = old.Namespace
		}
	}

	if envArgs.Namespace == "" {
		envArgs.Namespace = "default"
	}

	var message = ""
	// Check If Namespace Exists
	if err := c.Get(ctx, k8stypes.NamespacedName{Name: envArgs.Namespace}, &corev1.Namespace{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return message, err
		}
		// Create Namespace if not found
		if err := c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: envArgs.Namespace}}); err != nil {
			return message, err
		}
	}

	data, err := json.Marshal(envArgs)
	if err != nil {
		return message, err
	}
	envdir, err := system.GetEnvDir()
	if err != nil {
		return message, err
	}
	subEnvDir := filepath.Join(envdir, envName)
	if _, err = system.CreateIfNotExist(subEnvDir); err != nil {
		return message, err
	}
	// nolint:gosec
	if err = ioutil.WriteFile(filepath.Join(subEnvDir, system.EnvConfigName), data, 0644); err != nil {
		return message, err
	}
	curEnvPath, err := system.GetCurrentEnvPath()
	if err != nil {
		return message, err
	}
	// nolint:gosec
	if err = ioutil.WriteFile(curEnvPath, []byte(envName), 0644); err != nil {
		return message, err
	}

	message = fmt.Sprintf("environment %s %s, Namespace: %s", envName, createOrUpdated, envArgs.Namespace)
	if envArgs.Email != "" {
		message += fmt.Sprintf(", Email: %s", envArgs.Email)
	}
	return message, nil
}

// CreateEnv will only create. If env already exists, return error
func CreateEnv(ctx context.Context, c client.Client, envName string, envArgs *types.EnvMeta) (string, error) {
	_, err := GetEnvByName(envName)
	if err == nil {
		message := fmt.Sprintf("Env %s already exist", envName)
		return message, errors.New(message)
	}
	return CreateOrUpdateEnv(ctx, c, envName, envArgs)
}

// UpdateEnv will update Env, if env does not exist, return error
func UpdateEnv(ctx context.Context, c client.Client, envName string, namespace string) (string, error) {
	var message = ""
	envMeta, err := GetEnvByName(envName)
	if err != nil {
		return err.Error(), err
	}
	if err := c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: envMeta.Namespace}}); err != nil && !apierrors.IsAlreadyExists(err) {
		return message, err
	}
	envMeta.Namespace = namespace
	data, err := json.Marshal(envMeta)
	if err != nil {
		return message, err
	}
	envdir, err := system.GetEnvDir()
	if err != nil {
		return message, err
	}
	subEnvDir := filepath.Join(envdir, envName)
	// nolint:gosec
	if err = ioutil.WriteFile(filepath.Join(subEnvDir, system.EnvConfigName), data, 0644); err != nil {
		return message, err
	}
	message = "Update env succeed"
	return message, err
}

// ListEnvs will list all envs
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
	envDir, err := system.GetEnvDir()
	if err != nil {
		return envList, err
	}
	files, err := ioutil.ReadDir(envDir)
	if err != nil {
		return envList, err
	}
	curEnv, err := GetCurrentEnvName()
	if err != nil {
		curEnv = types.DefaultEnvName
	}
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		data, err := ioutil.ReadFile(filepath.Clean(filepath.Join(envDir, f.Name(), system.EnvConfigName)))
		if err != nil {
			continue
		}
		var envMeta types.EnvMeta
		if err = json.Unmarshal(data, &envMeta); err != nil {
			continue
		}
		if curEnv == f.Name() {
			envMeta.Current = "*"
		}
		envList = append(envList, &envMeta)
	}
	return envList, nil
}

// GetCurrentEnvName will get current env name
func GetCurrentEnvName() (string, error) {
	currentEnvPath, err := system.GetCurrentEnvPath()
	if err != nil {
		return "", err
	}
	data, err := ioutil.ReadFile(filepath.Clean(currentEnvPath))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// DeleteEnv will delete env locally
func DeleteEnv(envName string) (string, error) {
	var message string
	var err error
	curEnv, err := GetCurrentEnvName()
	if err != nil {
		return message, err
	}
	if envName == curEnv {
		err = fmt.Errorf("you can't delete current using environment %s", curEnv)
		return message, err
	}
	envdir, err := system.GetEnvDir()
	if err != nil {
		return message, err
	}
	envPath := filepath.Join(envdir, envName)
	if _, err := os.Stat(envPath); err != nil {
		if os.IsNotExist(err) {
			err = fmt.Errorf("%s does not exist", envName)
			return message, err
		}
	}
	if err = os.RemoveAll(envPath); err != nil {
		return message, err
	}
	message = envName + " deleted"
	return message, err
}

// SetEnv will set the current env to the specified one
func SetEnv(envName string) (string, error) {
	var msg string
	currentEnvPath, err := system.GetCurrentEnvPath()
	if err != nil {
		return msg, err
	}
	envMeta, err := GetEnvByName(envName)
	if err != nil {
		return msg, err
	}
	//nolint:gosec
	if err = ioutil.WriteFile(currentEnvPath, []byte(envName), 0644); err != nil {
		return msg, err
	}
	msg = fmt.Sprintf("Set environment succeed, current environment is " + envName + ", namespace is " + envMeta.Namespace)
	return msg, nil
}
