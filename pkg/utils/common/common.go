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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/encoding/openapi"
	"github.com/AlecAivazis/survey/v2"
	"github.com/ghodss/yaml"
	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	certmanager "github.com/wonderflow/cert-manager-api/pkg/apis/certmanager/v1"
	istioclientv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev"
	oamstandard "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	mycue "github.com/oam-dev/kubevela/pkg/cue"
)

var (
	// Scheme defines the default KubeVela schema
	Scheme = k8sruntime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(Scheme)
	_ = crdv1.AddToScheme(Scheme)
	_ = oamcore.AddToScheme(Scheme)
	_ = oamstandard.AddToScheme(Scheme)
	_ = istioclientv1beta1.AddToScheme(Scheme)
	_ = certmanager.AddToScheme(Scheme)
	_ = kruise.AddToScheme(Scheme)
	// +kubebuilder:scaffold:scheme
}

// InitBaseRestConfig will return reset config for create controller runtime client
func InitBaseRestConfig() (Args, error) {
	restConf, err := config.GetConfig()
	if err != nil {
		fmt.Println("get kubeConfig err", err)
		os.Exit(1)
	}

	return Args{
		Config: restConf,
		Schema: Scheme,
	}, nil
}

// HTTPGet will send GET http request with context
func HTTPGet(ctx context.Context, url string) ([]byte, error) {
	// Change NewRequest to NewRequestWithContext and pass context it
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

// GetCUEParameterValue converts definitions to cue format
func GetCUEParameterValue(cueStr string) (cue.Value, error) {
	r := cue.Runtime{}
	template, err := r.Compile("", cueStr+mycue.BaseTemplate)
	if err != nil {
		return cue.Value{}, err
	}
	tempStruct, err := template.Value().Struct()
	if err != nil {
		return cue.Value{}, err
	}
	// find the parameter definition
	var paraDef cue.FieldInfo
	var found bool
	for i := 0; i < tempStruct.Len(); i++ {
		paraDef = tempStruct.Field(i)
		if paraDef.Name == mycue.ParameterTag {
			found = true
			break
		}
	}
	if !found {
		return cue.Value{}, errors.New("parameter not exist")
	}
	arguments := paraDef.Value

	return arguments, nil
}

// GenOpenAPI generates OpenAPI json schema from cue.Instance
func GenOpenAPI(inst *cue.Instance) ([]byte, error) {
	if inst.Err != nil {
		return nil, inst.Err
	}
	defaultConfig := &openapi.Config{}
	b, err := openapi.Gen(inst, defaultConfig)
	if err != nil {
		return nil, err
	}
	var out = &bytes.Buffer{}
	_ = json.Indent(out, b, "", "   ")
	return out.Bytes(), nil
}

// RealtimePrintCommandOutput prints command output in real time
// If logFile is "", it will prints the stdout, or it will write to local file
func RealtimePrintCommandOutput(cmd *exec.Cmd, logFile string) error {
	var writer io.Writer
	if logFile == "" {
		writer = io.MultiWriter(os.Stdout)
	} else {
		if _, err := os.Stat(filepath.Dir(logFile)); err != nil {
			return err
		}
		f, err := os.Create(filepath.Clean(logFile))
		if err != nil {
			return err
		}
		writer = io.MultiWriter(f)
	}
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// AskToChooseOneService will ask users to select one service of the application if more than one exidi
func AskToChooseOneService(svcNames []string) (string, error) {
	if len(svcNames) == 0 {
		return "", fmt.Errorf("no service exist in the application")
	}
	if len(svcNames) == 1 {
		return svcNames[0], nil
	}
	prompt := &survey.Select{
		Message: "You have multiple services in your app. Please choose one service: ",
		Options: svcNames,
	}
	var svcName string
	err := survey.AskOne(prompt, &svcName)
	if err != nil {
		return "", fmt.Errorf("choosing service err %w", err)
	}
	return svcName, nil
}

// ReadYamlToObject will read a yaml K8s object to runtime.Object
func ReadYamlToObject(path string, object k8sruntime.Object) error {
	data, err := ioutil.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, object)
}
