/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

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
package cmd

import (
	"fmt"
	"io/ioutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
	"sigs.k8s.io/yaml"
	"strconv"

	oam "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/spf13/cobra"
)

var supportedFlags []string = []string{"port"}

var ExceptionMessage = "You must specify a Component workload, like `ContainerizedWorkload`, or ..." +
	"\n\nerror: Required OAM Component workload not specified." +
	"\nSee 'rudr run -h' for help and examples"

var WorkloadExceptionMessage = "You must specify a Component workload, like `ContainerizedWorkload`, or ..." +
	"\n\nerror: Required OAM Component workload not specified." +
	"\nSee 'rudr run -h' for help and examples"

// runCmd represents the run command
var RunCmd = &cobra.Command{
	Use: "run",
	Short: "Create an OAM component, or ...\n" +
		"Example: rudr run containerized frontend -p 80 oam-dev/demo:v1",
	Long: ``,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Println(ExceptionMessage)
		}
	},
}

// Use this new runCmd for better unit-test experience
func NewRunCmd(in string) *cobra.Command {
	return RunCmd
}

var containerizedSubCmd = &cobra.Command{
	Use:        "ContainerizedWorkload",
	Aliases:    []string{"containerized"},
	Short:      "Create a ContainerizedWorkload Component",
	SuggestFor: []string{"ContainerizedWorkload", "Deployment"},
	Long:       ``,
	Run: func(cmd *cobra.Command, args []string) {
		f := cmd.Flags()

		argsLenght := len(args)

		if argsLenght == 0 {
			fmt.Println("You must specify a name for ContainerizedWorkload" +
				"\n\nerror: Required the name for OAM Component workload ContainerizedWorkload" +
				"\nSee 'rudr run -h' for help and examples")
		} else if argsLenght < 2 {
			// TODO(zzxwill): Coud not determine whether the argument is ContainerizedWorkload name or image name without image tag
			fmt.Println("You must specify a name for ContainerizedWorkload, or image and its tag for ContainerizedWorkload" +
				"\n\nerror: Required ContainerizedWorkload name or image for OAM Component workload ContainerizedWorkload, like nginx:1.9.4" +
				"\nSee 'rudr run -h' for help and examples")
		} else if f.HasFlags() {
			// TODO(zzxwill): Need to check whether all of the flags have values
			// fmt.Println("containerized called")
			port := cmd.Flag("port")

			componentName := args[0]
			image := args[1]

			// fmt.Println(componentName, image, port)

			if port != nil {
				p, err := strconv.ParseInt(port.Value.String(), 10, 32)
				if err != nil {
					fmt.Println("Port is not in the right format.")
				}
				workload := oam.ContainerizedWorkload{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ContainerizedWorkload",
						APIVersion: "core.oam.dev/v1alpha2",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: componentName,
					},
					Spec: oam.ContainerizedWorkloadSpec{
						Containers: []oam.Container{
							oam.Container{
								Name:  componentName,
								Image: image,
								Ports: []oam.ContainerPort{
									oam.ContainerPort{
										Name: componentName,
										Port: int32(p),
									},
								},
							},
						},
					},
				}

				component := oam.Component{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Component",
						APIVersion: "core.oam.dev/v1alpha2",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: componentName,
					},
					Spec: oam.ComponentSpec{
						Workload: runtime.RawExtension{
							Object: &workload,
						},
					},
				}

				appconfig := oam.ApplicationConfiguration{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ApplicationConfiguration",
						APIVersion: "core.oam.dev/v1alpha2",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: componentName,
					},
					Spec: oam.ApplicationConfigurationSpec{
						Components: []oam.ApplicationConfigurationComponent{
							oam.ApplicationConfigurationComponent{
								ComponentName: componentName,
							}},
					},
				}

				c, err1 := yaml.Marshal(&component)
				a, err2 := yaml.Marshal(&appconfig)

				if err1 != nil || err2 != nil{
					fmt.Println("Failed to creat manifests for Component and ApplicationConfiguration.")
					return
				}

				f := fmt.Sprintf("./.rudr/.build/%s", componentName)
				os.MkdirAll(f, os.ModePerm)

				fileName := fmt.Sprintf("%s/component-%s.yaml", f, componentName)
				ioutil.WriteFile(fileName, c, 0644)

				fileName = fmt.Sprintf("%s/appconfig-%s.yaml", f, componentName)
				ioutil.WriteFile(fileName, a, 0644)

				fmt.Println("Successfully created manifests for Component and ApplicationConfiguration.")

			}

		}

	},
}

func init() {
	rootCmd.AddCommand(RunCmd)
	RunCmd.AddCommand(containerizedSubCmd)

	RunCmd.PersistentFlags().StringP("port", "p", "80", "Container port")
	// containerizedSubCmd.PersistentFlags().StringVarP(&userLicense, "license", "l", "", "Name of license for the project (can provide `licensetext` in config)")
	// containerizedSubCmd.PersistentFlags().Bool("viper", true, "Use Viper for configuration")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// runCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// runCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
