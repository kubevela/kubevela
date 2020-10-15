package oam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	acmev1 "github.com/wonderflow/cert-manager-api/pkg/apis/acme/v1"
	certmanager "github.com/wonderflow/cert-manager-api/pkg/apis/certmanager/v1"
	v1 "github.com/wonderflow/cert-manager-api/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

func GetEnvByName(name string) (*types.EnvMeta, error) {
	data, err := ioutil.ReadFile(filepath.Join(system.GetEnvDirByName(name), system.EnvConfigName))
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

//Create or update env.
//If it does not exist, create it and set to the new env.
//If it exists, update it and set to the new env.
func CreateOrUpdateEnv(ctx context.Context, c client.Client, envName string, envArgs *types.EnvMeta) (string, error) {
	var message = ""
	// Create Namespace
	if err := c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: envArgs.Namespace}}); err != nil && !apierrors.IsAlreadyExists(err) {
		return message, err
	}

	// Create Issuer For SSL
	if envArgs.Email != "" {
		issuerName := "oam-env-" + envArgs.Name
		if err := c.Create(ctx, &certmanager.Issuer{
			ObjectMeta: metav1.ObjectMeta{Name: issuerName, Namespace: envArgs.Namespace},
			Spec: certmanager.IssuerSpec{
				IssuerConfig: certmanager.IssuerConfig{
					ACME: &acmev1.ACMEIssuer{
						Email:  envArgs.Email,
						Server: "https://acme-v02.api.letsencrypt.org/directory",
						PrivateKey: v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{Name: "oam-env-" + envArgs.Name + ".key"},
						},
						Solvers: []acmev1.ACMEChallengeSolver{{
							HTTP01: &acmev1.ACMEChallengeSolverHTTP01{
								Ingress: &acmev1.ACMEChallengeSolverHTTP01Ingress{Class: GetStringPointer("nginx")},
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
	if err = ioutil.WriteFile(filepath.Join(subEnvDir, system.EnvConfigName), data, 0644); err != nil {
		return message, err
	}
	curEnvPath, err := system.GetCurrentEnvPath()
	if err != nil {
		return message, err
	}
	if err = ioutil.WriteFile(curEnvPath, []byte(envName), 0644); err != nil {
		return message, err
	}
	message = fmt.Sprintf("environment %s created, Namespace: %s, Email: %s.", envName, envArgs.Namespace, envArgs.Email)
	return message, nil
}

func GetStringPointer(v string) *string {
	return &v
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

//Update Env, if env does not exist, return error
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
	if err = ioutil.WriteFile(filepath.Join(subEnvDir, system.EnvConfigName), data, 0644); err != nil {
		return message, err
	}
	message = "Update env succeed"
	return message, err
}

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
		data, err := ioutil.ReadFile(filepath.Join(envDir, f.Name(), system.EnvConfigName))
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

func GetCurrentEnvName() (string, error) {
	currentEnvPath, err := system.GetCurrentEnvPath()
	if err != nil {
		return "", err
	}
	data, err := ioutil.ReadFile(currentEnvPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

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
	if err = ioutil.WriteFile(currentEnvPath, []byte(envName), 0644); err != nil {
		return msg, err
	}
	msg = fmt.Sprintf("Set environment succeed, current environment is " + envName + ", namespace is " + envMeta.Namespace)
	return msg, nil
}
