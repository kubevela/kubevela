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

package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/jsonpath"
	"k8s.io/kubectl/pkg/cmd/get"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
)

// UserInput user input in command
type UserInput struct {
	Writer io.Writer
	Reader *bufio.Reader
}

// UserInputOptions user input options
type UserInputOptions struct {
	AssumeYes bool
}

// NewUserInput new user input util
func NewUserInput() *UserInput {
	return &UserInput{
		Writer: os.Stdout,
		Reader: bufio.NewReader(os.Stdin),
	}
}

// AskBool format the answer to bool type
func (ui *UserInput) AskBool(question string, opts *UserInputOptions) bool {
	fmt.Fprintf(ui.Writer, "%s (y/n)", question)
	if opts.AssumeYes {
		return true
	}
	line, err := ui.read()
	if err != nil {
		log.Fatal(err.Error())
	}
	if input := strings.TrimSpace(strings.ToLower(line)); input == "y" || input == "yes" {
		return true
	}
	return false
}

func (ui *UserInput) read() (string, error) {
	line, err := ui.Reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	resultStr := strings.TrimSuffix(line, "\n")
	return resultStr, err
}

// formatApplicationString formats an Application to string in yaml/json/jsonpath for printing (without managedFields).
//
// format = "yaml" / "json" / "jsonpath={.field}"
func formatApplicationString(format string, app *v1beta1.Application) (string, error) {
	// No, we don't want managedFields, get rid of it.
	app.ManagedFields = nil

	return printObj(format, app)
}

// AskToChooseOnePod will ask user to select one pod
func AskToChooseOnePod(pods []types.PodBase) (*types.PodBase, error) {
	if len(pods) == 0 {
		return nil, errors.New("no pod found in your application")
	}
	if len(pods) == 1 {
		return &pods[0], nil
	}
	var ops []string
	for i := 0; i < len(pods); i++ {
		pod := pods[i]
		ops = append(ops, fmt.Sprintf("%s | %s | %s", pod.Cluster, pod.Component, pod.Metadata.Name))
	}
	prompt := &survey.Select{
		Message: fmt.Sprintf("There are %d pods match your filter conditions. Please choose one:\nCluster | Component | Pod", len(ops)),
		Options: ops,
	}
	var selectedRsc string
	err := survey.AskOne(prompt, &selectedRsc)
	if err != nil {
		return nil, fmt.Errorf("choosing pod err %w", err)
	}
	for k, resource := range ops {
		if selectedRsc == resource {
			return &pods[k], nil
		}
	}
	// it should never happen.
	return nil, errors.New("no pod match for your choice")
}

// AskToChooseOneService will ask user to select one service and/or port
func AskToChooseOneService(services []types.ResourceItem, selectPort bool) (*types.ResourceItem, int, error) {
	if len(services) == 0 {
		return nil, 0, errors.New("no service found in your application")
	}
	var ops []string
	var res []struct {
		item types.ResourceItem
		port int
	}
	for i := 0; i < len(services); i++ {
		obj := services[i]
		service := &corev1.Service{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object.Object, service); err == nil {
			if selectPort {
				for _, port := range service.Spec.Ports {
					ops = append(ops, fmt.Sprintf("%s | %s | %s:%d", obj.Cluster, obj.Component, obj.Object.GetName(), port.Port))
					res = append(res, struct {
						item types.ResourceItem
						port int
					}{
						item: obj,
						port: int(port.Port),
					})
				}
			} else {
				ops = append(ops, fmt.Sprintf("%s | %s | %s", obj.Cluster, obj.Component, obj.Object.GetName()))
				res = append(res, struct {
					item types.ResourceItem
					port int
				}{
					item: obj,
				})
			}

		}
	}
	if len(ops) == 1 {
		return &res[0].item, res[0].port, nil
	}
	prompt := &survey.Select{
		Message: fmt.Sprintf("There are %d services match your filter conditions. Please choose one:\nCluster | Component | Service", len(ops)),
		Options: ops,
	}
	var selectedRsc string
	err := survey.AskOne(prompt, &selectedRsc)
	if err != nil {
		return nil, 0, fmt.Errorf("choosing service err %w", err)
	}
	for k, resource := range ops {
		if selectedRsc == resource {
			return &res[k].item, res[k].port, nil
		}
	}
	// it should never happen.
	return nil, 0, errors.New("no service match for your choice")
}

func convertApplicationRevisionTo(format string, apprev *v1beta1.ApplicationRevision) (string, error) {
	// No, we don't want managedFields, get rid of it.
	apprev.ManagedFields = nil

	return printObj(format, apprev)
}

func printObj(format string, obj interface{}) (string, error) {
	var ret string

	if format == "" {
		return "", fmt.Errorf("no format provided")
	}

	switch format {
	case "yaml":
		b, err := yaml.Marshal(obj)
		if err != nil {
			return "", err
		}
		ret = string(b)
	case "json":
		b, err := json.MarshalIndent(obj, "", "  ")
		if err != nil {
			return "", err
		}
		ret = string(b)
	default:
		// format is not any of json/yaml/jsonpath, not supported
		if !strings.HasPrefix(format, "jsonpath") {
			return "", fmt.Errorf("%s is not supported", format)
		}

		// format = jsonpath
		s := strings.SplitN(format, "=", 2)
		if len(s) < 2 {
			return "", fmt.Errorf("jsonpath template format specified but no template given")
		}
		path, err := get.RelaxedJSONPathExpression(s[1])
		if err != nil {
			return "", err
		}

		jp := jsonpath.New("").AllowMissingKeys(true)
		err = jp.Parse(path)
		if err != nil {
			return "", err
		}

		buf := &bytes.Buffer{}
		err = jp.Execute(buf, obj)
		if err != nil {
			return "", err
		}
		ret = buf.String()
	}

	return ret, nil
}
