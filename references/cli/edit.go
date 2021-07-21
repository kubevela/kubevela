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
	"encoding/json"
	"fmt"
	"github.com/ghodss/yaml"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

func NewEditCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	var editTemplate bool
	var editor string
	var namespace string
	cmd := &cobra.Command{
		Use:   "edit [MODULE_TYPE:trait|component|...] [MODULE_NAME:ingress|webservice|...]",
		Short: "edit the configuration of a definition",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return errors.New("Hint: please specify exactly two parameters")
			}
			moduleType := args[0]
			moduleName := args[1]
			env, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			if namespace == "" {
				namespace = env.Namespace
			}
			return edit(ioStreams, namespace, moduleType, moduleName, editTemplate, editor)
		},
	}
	cmd.Flags().BoolVarP(&editTemplate, "edit-template", "", false, "specify this parameter to edit cue template")
	cmd.Flags().StringVarP(&editor, "editor", "", "vim", "specify the editor to use, for example, vi/vim/nano/<your editor path>")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "specify the target namespace")
	return cmd
}

// The edit function will first try to save the current configuration in a temporary file, then call editor to edit this file. If file changed, the function will call kubectl apply to update the target.
func edit(ioStreams cmdutil.IOStreams, namespace string, moduleType string, moduleName string, editTemplate bool, editor string) error {
	tmpDir, err := ioutil.TempDir(os.TempDir(), "vela-*")
	if err != nil {
		return errors.Wrap(err, "Create temporary directory failed")
	}
	defer func(dirname string) {
		if err := os.RemoveAll(dirname); err != nil {
			ioStreams.Errorf("Failed to remove temporary dir %s, err: %v\n", dirname, err)
		}
	}(tmpDir)
	cueTemplateFilepath := tmpDir + "/" + moduleName + ".cue"
	jsonOut, err := exec.Command("kubectl", "get", moduleType+"definitions.core.oam.dev", moduleName, "--namespace", namespace, "-o", "json").CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to get %s with kubectl: %s\n", moduleName, string(jsonOut))
	}
	obj, ok := gjson.ParseBytes(jsonOut).Value().(map[string]interface{})
	if !ok {
		return errors.New("Failed to get target during parsing JSON")
	}
	changed := false
	runEditor := func(filepath string) (string, error) {
		editScriptPath := tmpDir + "/edit.sh"
		if err := ioutil.WriteFile(editScriptPath, []byte(editor+" "+filepath), 0644); err != nil {
			return "", errors.Wrapf(err, "Failed to create edit script at %s", editScriptPath)
		}
		editCmd := exec.Command("sh", editScriptPath)
		editCmd.Stdin = os.Stdin
		editCmd.Stdout = os.Stdout
		editCmd.Stderr = os.Stderr
		if err = editCmd.Run(); err != nil {
			return "", errors.Wrapf(err, "Failed to run editor %s at path %s", editor, filepath)
		}
		buf, err := ioutil.ReadFile(filepath)
		if err != nil {
			return "", errors.Wrapf(err, "Failed to read temporary file %s", filepath)
		}
		return string(buf), nil
	}
	if editTemplate {
		cueTemplateBytes, err := exec.Command("kubectl", "get", moduleType+"definitions.core.oam.dev", moduleName, "--namespace", namespace, "-o", "jsonpath='{.spec.schematic.cue.template}'").CombinedOutput()
		if err != nil {
			return fmt.Errorf("Failed to get the template of %s with kubectl: %s\n", moduleName, string(cueTemplateBytes))
		}
		cueTemplateContent := strings.TrimSpace(strings.Trim(string(cueTemplateBytes), "'"))
		if err := ioutil.WriteFile(cueTemplateFilepath, []byte(cueTemplateContent), 0644); err != nil {
			return errors.Wrapf(err, "Failed to write target template CUE into temporary file %s\n", cueTemplateFilepath)
		}
		newCueTemplateContent, err := runEditor(cueTemplateFilepath)
		if err != nil {
			return err
		}
		if cueTemplateContent != newCueTemplateContent {
			changed = true
			obj["spec"].(map[string]interface{})["schematic"].(map[string]interface{})["cue"].(map[string]interface{})["template"] = newCueTemplateContent
		}
	} else {
		templateCue := obj["spec"].(map[string]interface{})["schematic"].(map[string]interface{})["cue"].(map[string]interface{})["template"]
		obj["spec"].(map[string]interface{})["schematic"].(map[string]interface{})["cue"].(map[string]interface{})["template"] = "<Edit this field using the --edit-template flag>"
		yamlOut, err := yaml.Marshal(obj)
		if err != nil {
			return errors.Wrap(err, "Failed to marshal YAML of target")
		}
		yamlFilepath := tmpDir + "/" + moduleName + ".yaml"
		if err := ioutil.WriteFile(yamlFilepath, yamlOut, 0644); err != nil {
			return errors.Wrapf(err, "Failed to write target YAML into temporary file %s\n", yamlFilepath)
		}
		newYamlTargetContent, err := runEditor(yamlFilepath)
		if err != nil {
			return err
		}
		if newYamlTargetContent != string(yamlOut) {
			changed = true
			if err = yaml.Unmarshal([]byte(newYamlTargetContent), &obj); err != nil {
				return errors.Wrap(err, "Failed to unmarshal edited target YAML")
			}
			obj["spec"].(map[string]interface{})["schematic"].(map[string]interface{})["cue"].(map[string]interface{})["template"] = templateCue
		}
	}
	if changed {
		if jsonOut, err = json.Marshal(obj); err != nil {
			return errors.Wrap(err, "Failed to marshal changed target into JSON")
		}
		jsonOutFilepath := tmpDir + "/" + moduleName + ".json"
		if err = ioutil.WriteFile(jsonOutFilepath, jsonOut, 0644); err != nil {
			return errors.Wrapf(err, "Failed to write edited target into JSON temporary file %s", jsonOutFilepath)
		}
		if err = exec.Command("kubectl", "apply", "-f", jsonOutFilepath).Run(); err != nil {
			return errors.Wrapf(err, "Failed to apply edited target")
		}
		ioStreams.Info("Target Updated.")
	} else {
		ioStreams.Info("Target Unchanged.")
	}
	return nil
}
