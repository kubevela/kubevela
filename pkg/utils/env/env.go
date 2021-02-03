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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"

	"github.com/pkg/errors"
	acmev1 "github.com/wonderflow/cert-manager-api/pkg/apis/acme/v1"
	certmanager "github.com/wonderflow/cert-manager-api/pkg/apis/certmanager/v1"
	v1 "github.com/wonderflow/cert-manager-api/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// CurrentEnvLabel is the current using environment
	CurrentEnvLabel = "env.core.oam.dev/is-default"

	// ProductionACMEServer is the production ACME Server from let's encrypt
	ProductionACMEServer = "https://acme-v02.api.letsencrypt.org/directory"
)

// GetEnvByName will get env info by name
func GetEnvByName(ctx context.Context, c client.Client, name string) (*types.EnvMeta, error) {
	var env v1alpha2.Environment
	err := c.Get(ctx, client.ObjectKey{Name: name}, &env)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("env %s not exist", name)
		}
		return nil, err
	}
	return convertEnvironmenttoEnvMeta(env), nil
}

// CreateOrUpdateEnv will create or update env.
// If it does not exist, create it and set to the new env.
// If it exists, update it and set to the new env.
func CreateOrUpdateEnv(ctx context.Context, c client.Client, envArgs *types.EnvMeta) (string, error) {
	createOrUpdated := "created"
	old, err := GetEnvByName(ctx, c, envArgs.Name)
	if err == nil {
		createOrUpdated = "updated"
		if envArgs.Domain == "" {
			envArgs.Domain = old.Domain
		}
		if envArgs.Email == "" {
			envArgs.Email = old.Email
		}
		if envArgs.Issuer == "" {
			envArgs.Issuer = old.Issuer
		}
		if envArgs.Namespace == "" {
			envArgs.Namespace = old.Namespace
		}
		if envArgs.Current == "" {
			envArgs.Current = old.Current
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

	// Create Issuer For SSL if both email and domain are all set.
	if envArgs.Email != "" && envArgs.Domain != "" {
		issuerName := "oam-env-" + envArgs.Name
		if err := c.Create(ctx, &certmanager.Issuer{
			ObjectMeta: metav1.ObjectMeta{Name: issuerName, Namespace: envArgs.Namespace},
			Spec: certmanager.IssuerSpec{
				IssuerConfig: certmanager.IssuerConfig{
					ACME: &acmev1.ACMEIssuer{
						Email:  envArgs.Email,
						Server: ProductionACMEServer,
						PrivateKey: v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{Name: "oam-env-" + envArgs.Name + ".key"},
						},
						Solvers: []acmev1.ACMEChallengeSolver{{
							HTTP01: &acmev1.ACMEChallengeSolverHTTP01{
								Ingress: &acmev1.ACMEChallengeSolverHTTP01Ingress{Class: pointer.StringPtr("nginx")},
							},
						}},
					},
				},
			},
		}); err != nil && !apierrors.IsAlreadyExists(err) {
			return message, err
		}
		envArgs.Issuer = issuerName
	}
	if createOrUpdated == "created" {
		if _, err = CreateEnv(ctx, c, envArgs); err != nil {
			return message, err
		}
	} else {
		if _, err = UpdateEnv(ctx, c, envArgs); err != nil {
			return message, err
		}
	}

	message = fmt.Sprintf("environment %s %s, Namespace: %s", envArgs.Name, createOrUpdated, envArgs.Namespace)
	if envArgs.Email != "" {
		message += fmt.Sprintf(", Email: %s", envArgs.Email)
	}
	if _, err := SetEnv(ctx, c, envArgs.Name); err != nil {
		return message, err
	}
	return message, nil
}

// CreateEnv will only create. If env already exists, return error
func CreateEnv(ctx context.Context, c client.Client, envArgs *types.EnvMeta) (string, error) {
	err := c.Create(ctx, convertEnvMetatoEnvironment(*envArgs))
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			message := fmt.Sprintf("Env %s already exist", envArgs.Name)
			return message, err
		}
		return err.Error(), err
	}
	return fmt.Sprintf("environment %s created, Namespace: %s", envArgs.Name, envArgs.Namespace), nil
}

// UpdateEnv will update Env, if env does not exist, return error
func UpdateEnv(ctx context.Context, c client.Client, envArgs *types.EnvMeta) (string, error) {
	if err := c.Patch(ctx, convertEnvMetatoEnvironment(*envArgs), client.Merge); err != nil {
		return err.Error(), err
	}
	return "Update env succeed", nil
}

// ListEnvs will list all envs
func ListEnvs(ctx context.Context, c client.Client, envName string) ([]*types.EnvMeta, error) {
	var envList []*types.EnvMeta
	if envName != "" {
		env, err := GetEnvByName(ctx, c, envName)
		if err != nil {
			return envList, err
		}
		envList = append(envList, env)
		return envList, err
	}
	var environmentList v1alpha2.EnvironmentList
	err := c.List(context.Background(), &environmentList)
	if err != nil {
		return nil, err
	}
	for _, item := range environmentList.Items {
		envList = append(envList, convertEnvironmenttoEnvMeta(item))
	}
	return envList, nil
}

// GetCurrentEnv will get current env name
func GetCurrentEnv(ctx context.Context, c client.Client) (*types.EnvMeta, error) {
	currentEnv, err := getCurrentEnv(ctx, c)
	if err != nil {
		return nil, err
	}
	return convertEnvironmenttoEnvMeta(*currentEnv), nil
}

// DeleteEnv will delete env locally
func DeleteEnv(ctx context.Context, c client.Client, envName string) (string, error) {
	var message string
	curEnv, err := GetCurrentEnv(ctx, c)
	if err != nil {
		return message, err
	}
	if envName == curEnv.Name {
		err = fmt.Errorf("you can't delete current using environment %s", curEnv.Name)
		return message, err
	}
	err = c.Delete(ctx, &v1alpha2.Environment{ObjectMeta: metav1.ObjectMeta{
		Name: envName,
	}})
	if err != nil {
		return message, err
	}
	return fmt.Sprintf("environment %s deleted", envName), nil
}

// SetEnv will set the current env to the specified one
func SetEnv(ctx context.Context, c client.Client, envName string) (string, error) {
	var msg string
	var env v1alpha2.Environment
	if err := c.Get(ctx, client.ObjectKey{Name: envName}, &env); err != nil {
		return msg, err
	}

	currentEnv, err := getCurrentEnv(ctx, c)
	if err != nil && !apierrors.IsNotFound(err) {
		return msg, err
	}
	if currentEnv != nil && currentEnv.Name != envName {
		labels := currentEnv.GetLabels()
		delete(labels, CurrentEnvLabel)
		currentEnv.SetLabels(labels)
		err := c.Update(ctx, currentEnv)
		if err != nil {
			return msg, err
		}
	}
	labels := env.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[CurrentEnvLabel] = "true"
	env.SetLabels(labels)
	err = c.Update(ctx, &env)
	if err != nil {
		return msg, err
	}

	msg = fmt.Sprintf("Set environment succeed, current environment is " + envName + ", namespace is " + env.Spec.Namespace)
	return msg, nil
}

// InitDefaultEnv create env if not exits
func InitDefaultEnv(ctx context.Context, c client.Client) error {
	if _, err := getCurrentEnv(ctx, c); err == nil {
		return nil
	}
	_, err := CreateOrUpdateEnv(ctx, c, &types.EnvMeta{
		Name:      types.DefaultEnvName,
		Namespace: types.DefaultAppNamespace,
		Current:   "*",
	})
	return err
}

func convertEnvironmenttoEnvMeta(env v1alpha2.Environment) *types.EnvMeta {
	meta := types.EnvMeta{
		Name:      env.Name,
		Namespace: env.Spec.Namespace,
		Email:     env.Spec.Email,
		Domain:    env.Spec.Domain,
		Issuer:    env.Spec.Issuer,
	}
	if _, ok := env.ObjectMeta.Labels[CurrentEnvLabel]; ok {
		meta.Current = "*"
	}
	return &meta
}

func convertEnvMetatoEnvironment(meta types.EnvMeta) *v1alpha2.Environment {
	labels := make(map[string]string)
	if meta.Current == "*" {
		labels[CurrentEnvLabel] = "true"
	}
	env := v1alpha2.Environment{
		ObjectMeta: metav1.ObjectMeta{Name: meta.Name, Labels: labels},
		Spec: v1alpha2.EnvironmentSpec{
			Namespace:      meta.Namespace,
			Email:          meta.Email,
			Domain:         meta.Domain,
			Issuer:         meta.Issuer,
			CapabilityList: []string{},
		},
	}
	return &env
}

func getCurrentEnv(ctx context.Context, c client.Client) (*v1alpha2.Environment, error) {
	var envList v1alpha2.EnvironmentList
	_ = c.List(ctx, &envList, client.MatchingLabels{CurrentEnvLabel: "true"})
	if len(envList.Items) == 0 {
		return nil, errors.New("current env not set")
	}
	return &envList.Items[0], nil
}
