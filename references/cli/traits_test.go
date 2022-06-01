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
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	util2 "github.com/oam-dev/kubevela/pkg/oam/util"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

func TestTraitsAppliedToAllWorkloads(t *testing.T) {
	trait := types.Capability{
		Name:      "route",
		CrdName:   "routes.oam.dev",
		AppliesTo: []string{"*"},
	}
	workloads := []types.Capability{
		{
			Name:    "deployment",
			CrdName: "deployments.apps",
		},
		{
			Name:    "clonset",
			CrdName: "clonsets.alibaba",
		},
	}
	assert.Equal(t, []string{"*"}, common.ConvertApplyTo(trait.AppliesTo, workloads))
}

var _ = Describe("Test trait cli", func() {

	When("there are container-image and configmap traits", func() {
		BeforeEach(func() {
			// Install trait locally
			containerImage := v1beta1.TraitDefinition{}
			Expect(yaml.Unmarshal([]byte(containerImageYaml), &containerImage)).Should(BeNil())
			Expect(k8sClient.Create(context.Background(), &containerImage)).Should(SatisfyAny(BeNil(), util2.AlreadyExistMatcher{}))

			configMap := v1beta1.TraitDefinition{}
			Expect(yaml.Unmarshal([]byte(configmapYaml), &configMap)).Should(BeNil())
			Expect(k8sClient.Create(context.Background(), &configMap)).Should(SatisfyAny(BeNil(), util2.AlreadyExistMatcher{}))
		})

		It("should not have any err", func() {
			arg := common2.Args{}
			arg.SetClient(k8sClient)
			buffer := bytes.NewBuffer(nil)
			ioStreams := util.IOStreams{In: os.Stdin, Out: buffer, ErrOut: buffer}
			cmd := NewTraitCommand(arg, ioStreams)
			Expect(cmd.Execute()).Should(BeNil())
			buf, ok := ioStreams.Out.(*bytes.Buffer)
			Expect(ok).Should(BeTrue())
			Expect(strings.Contains(buf.String(), "error")).Should(BeFalse())
		})
	})
})

const (
	containerImageYaml = `apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: Set the image of the container.
  name: container-image
  namespace: vela-system
spec:
  appliesToWorkloads:
    - '*'
  podDisruptive: true
  schematic:
    cue:
      template: |
        #PatchParams: {
        	// +usage=Specify the name of the target container, if not set, use the component name
        	containerName: *"" | string
        	// +usage=Specify the image of the container
        	image: string
        	// +usage=Specify the image pull policy of the container
        	imagePullPolicy: *"" | "IfNotPresent" | "Always" | "Never"
        }
        PatchContainer: {
        	_params:         #PatchParams
        	name:            _params.containerName
        	_baseContainers: context.output.spec.template.spec.containers
        	_matchContainers_: [ for _container_ in _baseContainers if _container_.name == name {_container_}]
        	_baseContainer: *_|_ | {...}
        	if len(_matchContainers_) == 0 {
        		err: "container \(name) not found"
        	}
        	if len(_matchContainers_) > 0 {
        		// +patchStrategy=retainKeys
        		image: _params.image

        		if _params.imagePullPolicy != "" {
        			// +patchStrategy=retainKeys
        			imagePullPolicy: _params.imagePullPolicy
        		}
        	}
        }
        patch: spec: template: spec: {
        	if parameter.containers == _|_ {
        		// +patchKey=name
        		containers: [{
        			PatchContainer & {_params: {
        				if parameter.containerName == "" {
        					containerName: context.name
        				}
        				if parameter.containerName != "" {
        					containerName: parameter.containerName
        				}
        				image:           parameter.image
        				imagePullPolicy: parameter.imagePullPolicy
        			}}
        		}]
        	}
        	if parameter.containers != _|_ {
        		// +patchKey=name
        		containers: [ for c in parameter.containers {
        			if c.containerName == "" {
        				err: "containerName must be set for containers"
        			}
        			if c.containerName != "" {
        				PatchContainer & {_params: c}
        			}
        		}]
        	}
        }
        parameter: *#PatchParams | close({
        	// +usage=Specify the container image for multiple containers
        	containers: [...#PatchParams]
        })
        errs: [ for c in patch.spec.template.spec.containers if c.err != _|_ {c.err}]
`
	configmapYaml = `apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: Create/Attach configmaps on K8s pod for your workload which follows the pod spec in path 'spec.template'. This definition is DEPRECATED, please specify configmap in 'storage' instead.
  labels:
    custom.definition.oam.dev/deprecated: "true"
  name: configmap
  namespace: vela-system
spec:
  appliesToWorkloads:
    - '*'
  podDisruptive: true
  schematic:
    cue:
      template: |
        patch: spec: template: spec: {
        	containers: [{
        		// +patchKey=name
        		volumeMounts: [
        			for v in parameter.volumes {
        				{
        					name:      "volume-\(v.name)"
        					mountPath: v.mountPath
        					readOnly:  v.readOnly
        				}
        			},
        		]
        	}, ...]
        	// +patchKey=name
        	volumes: [
        		for v in parameter.volumes {
        			{
        				name: "volume-\(v.name)"
        				configMap: name: v.name
        			}
        		},
        	]
        }
        outputs: {
        	for v in parameter.volumes {
        		if v.data != _|_ {
        			"\(v.name)": {
        				apiVersion: "v1"
        				kind:       "ConfigMap"
        				metadata: name: v.name
        				data: v.data
        			}
        		}
        	}
        }
        parameter: {
        	// +usage=Specify mounted configmap names and their mount paths in the container
        	volumes: [...{
        		name:      string
        		mountPath: string
        		readOnly:  *false | bool
        		data?: [string]: string
        	}]
        }
`
)
