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

package appfile

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	util2 "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	// TerraformBaseLocation is the base directory to store all Terraform JSON files
	TerraformBaseLocation = ".vela/terraform/"
	// TerraformLog is the logfile name for terraform
	TerraformLog = "terraform.log"
)

// ApplyTerraform deploys addon resources
func ApplyTerraform(app *v1beta1.Application, k8sClient client.Client, ioStream util.IOStreams, namespace string, args common.Args) ([]commontypes.ApplicationComponent, error) {
	dm, err := args.GetDiscoveryMapper()
	if err != nil {
		return nil, err
	}
	pd, err := args.GetPackageDiscover()
	if err != nil {
		return nil, err
	}

	// TODO(zzxwill) Need to check whether authentication credentials of a specific cloud provider are exported as environment variables, like `ALICLOUD_ACCESS_KEY`
	var nativeVelaComponents []commontypes.ApplicationComponent
	// parse template
	appParser := appfile.NewApplicationParser(k8sClient, dm, pd)

	ctx := util2.SetNamespaceInCtx(context.Background(), namespace)
	appFile, err := appParser.GenerateAppFile(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("failed to parse appfile: %w", err)
	}
	if appFile == nil {
		return nil, fmt.Errorf("failed to parse appfile")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	revisionName, _ := utils.GetAppNextRevision(app)

	for i, wl := range appFile.Workloads {
		switch wl.CapabilityCategory {
		case types.TerraformCategory:
			name := wl.Name
			ioStream.Infof("\nApplying cloud resources %s\n", name)

			tf, err := getTerraformJSONFiles(wl, appFile.Name, revisionName, namespace)
			if err != nil {
				return nil, fmt.Errorf("failed to get Terraform JSON files from workload %s: %w", name, err)
			}

			tfJSONDir := filepath.Join(TerraformBaseLocation, name)
			if _, err = os.Stat(tfJSONDir); err != nil && os.IsNotExist(err) {
				if err = os.MkdirAll(tfJSONDir, 0750); err != nil {
					return nil, fmt.Errorf("failed to create directory for %s: %w", tfJSONDir, err)
				}
			}
			if err := os.WriteFile(filepath.Join(tfJSONDir, "main.tf.json"), tf, 0600); err != nil {
				return nil, fmt.Errorf("failed to convert Terraform template: %w", err)
			}

			outputs, err := callTerraform(tfJSONDir)
			if err != nil {
				return nil, err
			}
			if err := os.Chdir(cwd); err != nil {
				return nil, err
			}

			outputList := strings.Split(strings.ReplaceAll(string(outputs), " ", ""), "\n")
			if outputList[len(outputList)-1] == "" {
				outputList = outputList[:len(outputList)-1]
			}
			if err := generateSecretFromTerraformOutput(k8sClient, outputList, name, namespace); err != nil {
				return nil, err
			}
		default:
			nativeVelaComponents = append(nativeVelaComponents, app.Spec.Components[i])
		}

	}
	return nativeVelaComponents, nil
}

func callTerraform(tfJSONDir string) ([]byte, error) {
	if err := os.Chdir(tfJSONDir); err != nil {
		return nil, err
	}
	var cmd *exec.Cmd
	cmd = exec.Command("bash", "-c", "terraform init")
	if err := common.RealtimePrintCommandOutput(cmd, TerraformLog); err != nil {
		return nil, err
	}

	cmd = exec.Command("bash", "-c", "terraform apply --auto-approve")
	if err := common.RealtimePrintCommandOutput(cmd, TerraformLog); err != nil {
		return nil, err
	}

	// Get output from Terraform
	cmd = exec.Command("bash", "-c", "terraform output")
	outputs, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return outputs, nil
}

// generateSecretFromTerraformOutput generates secret from Terraform output
func generateSecretFromTerraformOutput(k8sClient client.Client, outputList []string, name, namespace string) error {
	ctx := context.TODO()
	err := k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
	if err == nil {
		return fmt.Errorf("namespace %s doesn't exist", namespace)
	}
	var cmData = make(map[string]string, len(outputList))
	for _, i := range outputList {
		line := strings.Split(i, "=")
		if len(line) != 2 {
			return fmt.Errorf("terraform output isn't in the right format")
		}
		k := strings.TrimSpace(line[0])
		v := strings.TrimSpace(line[1])
		if k != "" && v != "" {
			cmData[k] = v
		}
	}

	objectKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	var secret v1.Secret
	if err := k8sClient.Get(ctx, objectKey, &secret); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("retrieving the secret from cloud resource %s hit an issue: %w", name, err)
	} else if err == nil {
		if err := k8sClient.Delete(ctx, &secret); err != nil {
			return fmt.Errorf("failed to store cloud resource %s output to secret: %w", name, err)
		}
	}

	secret = v1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		StringData: cmData,
	}

	if err := k8sClient.Create(ctx, &secret); err != nil {
		return fmt.Errorf("failed to store cloud resource %s output to secret: %w", name, err)
	}
	return nil
}

// getTerraformJSONFiles gets Terraform JSON files or modules from workload
func getTerraformJSONFiles(wl *appfile.Workload, applicationName, revisionName string, namespace string) ([]byte, error) {
	pCtx, err := appfile.PrepareProcessContext(wl, applicationName, revisionName, namespace, nil)
	if err != nil {
		return nil, err
	}
	base, _ := pCtx.Output()
	tf, err := base.Compile()
	if err != nil {
		return nil, err
	}
	return tf, nil
}
