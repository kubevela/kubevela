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
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/pkg/strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

const (
	// IndicatingLabel is label key indicating application is an env
	IndicatingLabel = "cli.env.oam.dev/name"
	// RawType is component type of raw
	RawType = "raw"
	// DefaultEnvName is name of default env
	DefaultEnvName = "default"
	// DefaultEnvNamespace is namespace of default env
	DefaultEnvNamespace = "default"
	// AppNameSchema is used to generate env app name
	AppNameSchema = "vela-env-%s"
	// AppNamePrefix is prefix of AppNameSchema
	AppNamePrefix = "vela-env-"
)

// app2Env and env2App are helper convert functions
func app2Env(app *v1beta1.Application) (*types.EnvMeta, error) {
	namespace := getEnvNamespace(app)
	env := types.EnvMeta{
		Name:      strings.Replace(app.Name, AppNamePrefix, "", 1),
		Namespace: namespace,
		Current:   "",
	}
	return &env, nil
}

func env2App(meta *types.EnvMeta) *v1beta1.Application {
	app := v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1beta1.ApplicationKind,
			APIVersion: v1beta1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(AppNameSchema, meta.Name),
			Namespace: types.DefaultKubeVelaNS,
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common2.ApplicationComponent{},
		},
	}

	addNamespaceObjectIfNeeded(meta, &app)
	labels := map[string]string{
		IndicatingLabel: meta.Name,
	}
	app.SetLabels(labels)

	return &app
}

// getEnvAppByName and deleteAppByName are application operation helper functions
func getEnvAppByName(envName string) (*v1beta1.Application, error) {
	list, err := getEnvAppList()
	if err != nil {
		return nil, err
	}
	// match envName
	for _, app := range list {
		if app.Name == fmt.Sprintf(AppNameSchema, envName) {
			return &app, nil
		}
	}
	if envName == DefaultEnvName {
		_ = initDefaultEnv()
		return getEnvAppByName(envName)
	}
	return nil, errors.Errorf("application %s not found", envName)
}

func deleteAppByName(name string) error {
	clt, err := common.GetClient()
	if err != nil {
		return errors.Wrap(err, "get client fail")
	}
	app, err := getEnvAppByName(name)
	if err != nil {
		return err
	}
	err = clt.Delete(context.Background(), app)
	if err != nil {
		return err
	}
	return nil
}

// following functions are CRUD of env

// CreateEnv will create e env.
// Because Env equals to namespace, one env should not be updated
func CreateEnv(envName string, envArgs *types.EnvMeta) error {
	c, err := common.GetClient()
	if err != nil {
		return err
	}
	e := envArgs
	nowEnv, err := GetEnvByName(envName)
	if err == nil {
		if nowEnv.Namespace != envArgs.Namespace {
			return errors.Errorf("env %s has existed", envName)
		}
		return nil
	}
	if e.Namespace == "" {
		e.Namespace = "default"
	}

	app := env2App(envArgs)
	err = applyApp(context.TODO(), app, c)
	if err != nil {
		return err
	}
	return nil

}

// GetEnvByName will get env info by name
func GetEnvByName(name string) (*types.EnvMeta, error) {
	init, err := getEnvAppByName(name)
	if err != nil {
		return nil, errors.Wrap(err, "Env not exist")
	}
	return app2Env(init)
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
	apps, err := getEnvAppList()
	if err != nil {
		return nil, err
	}
	// if even one env is not exist, create a default env
	if len(apps) == 0 {
		err := initDefaultEnv()
		if err != nil {
			return nil, err
		}
		return ListEnvs(envName)
	}

	curEnv, err := getCurrentEnvName()
	if err != nil {
		curEnv = types.DefaultEnvName
	}
	for i := range apps {
		env, err := app2Env(&apps[i])
		if err != nil {
			fmt.Printf("error occurred while listing env:" + err.Error())
			continue
		}
		if curEnv == env.Name {
			env.Current = "*"
		}
		envList = append(envList, env)
	}
	return envList, nil
}

// DeleteEnv will delete env and its application
func DeleteEnv(envName string) (string, error) {
	var message string
	var err error
	curEnv, err := getCurrentEnvName()
	if err != nil {
		return message, err
	}
	if envName == curEnv {
		err = fmt.Errorf("you can't delete current using environment %s", curEnv)
		return message, err
	}
	err = deleteAppByName(envName)
	if err != nil {
		return message, errors.Wrap(err, fmt.Sprintf("error deleting env %s", envName))
	}
	message = "env" + envName + " deleted"
	return message, err
}

// getCurrentEnvName will get current env name
func getCurrentEnvName() (string, error) {
	currentEnvPath, err := system.GetCurrentEnvPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Clean(currentEnvPath))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetCurrentEnv will get current env, create default env if not exist
func GetCurrentEnv() (*types.EnvMeta, error) {
	envName, err := getCurrentEnvName()
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err = initDefaultEnv(); err != nil {
			return nil, err
		}
		envName = types.DefaultEnvName
	}
	return GetEnvByName(envName)
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
	if err = os.WriteFile(currentEnvPath, []byte(envName), 0644); err != nil {
		return msg, err
	}
	msg = fmt.Sprintf("Set environment succeed, current environment is " + envName + ", namespace is " + envMeta.Namespace)
	return msg, nil
}

// applyApp helps apply app
func applyApp(ctx context.Context, app *v1beta1.Application, c client.Client) error {
	applicator := apply.NewAPIApplicator(c)
	err := applicator.Apply(ctx, app)
	return err
}

// initDefaultEnv create default env if not exist
func initDefaultEnv() error {
	fmt.Println("Initializing default vela env...")
	defaultEnv := &types.EnvMeta{Name: DefaultEnvName, Namespace: DefaultEnvNamespace}
	app := env2App(defaultEnv)
	clt, err := common.GetClient()
	if err != nil {
		return err
	}
	err = applyApp(context.Background(), app, clt)
	if err != nil {
		return err
	}
	currentEnvPath, err := system.GetCurrentEnvPath()
	if err != nil {
		return err
	}
	//nolint:gosec
	if err = os.WriteFile(currentEnvPath, []byte(DefaultEnvName), 0644); err != nil {
		return err
	}
	return nil
}
