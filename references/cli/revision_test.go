/*
 Copyright 2022 The KubeVela Authors.

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
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kubevela/pkg/util/compression"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var compDef string = `apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  annotations:
    definition.oam.dev/description: Describes long-running, scalable, containerized
      services that have a stable network endpoint to receive external network traffic
      from customers.
    meta.helm.sh/release-name: kubevela
    meta.helm.sh/release-namespace: vela-system
  creationTimestamp: null
  labels:
    app.kubernetes.io/managed-by: Helm
  name: webservice
  namespace: vela-system
spec:
  schematic:
    cue:
      template: "import (\n\t\"strconv\"\n)\n\nmountsArray: {\n\tpvc: *[\n\t\tfor
        v in parameter.volumeMounts.pvc {\n\t\t\t{\n\t\t\t\tmountPath: v.mountPath\n\t\t\t\tif
        v.subPath != _|_ {\n\t\t\t\t\tsubPath: v.subPath\n\t\t\t\t}\n\t\t\t\tname:
        v.name\n\t\t\t}\n\t\t},\n\t] | []\n\n\tconfigMap: *[\n\t\t\tfor v in parameter.volumeMounts.configMap
        {\n\t\t\t{\n\t\t\t\tmountPath: v.mountPath\n\t\t\t\tif v.subPath != _|_ {\n\t\t\t\t\tsubPath:
        v.subPath\n\t\t\t\t}\n\t\t\t\tname: v.name\n\t\t\t}\n\t\t},\n\t] | []\n\n\tsecret:
        *[\n\t\tfor v in parameter.volumeMounts.secret {\n\t\t\t{\n\t\t\t\tmountPath:
        v.mountPath\n\t\t\t\tif v.subPath != _|_ {\n\t\t\t\t\tsubPath: v.subPath\n\t\t\t\t}\n\t\t\t\tname:
        v.name\n\t\t\t}\n\t\t},\n\t] | []\n\n\temptyDir: *[\n\t\t\tfor v in parameter.volumeMounts.emptyDir
        {\n\t\t\t{\n\t\t\t\tmountPath: v.mountPath\n\t\t\t\tif v.subPath != _|_ {\n\t\t\t\t\tsubPath:
        v.subPath\n\t\t\t\t}\n\t\t\t\tname: v.name\n\t\t\t}\n\t\t},\n\t] | []\n\n\thostPath:
        *[\n\t\t\tfor v in parameter.volumeMounts.hostPath {\n\t\t\t{\n\t\t\t\tmountPath:
        v.mountPath\n\t\t\t\tif v.subPath != _|_ {\n\t\t\t\t\tsubPath: v.subPath\n\t\t\t\t}\n\t\t\t\tname:
        v.name\n\t\t\t}\n\t\t},\n\t] | []\n}\nvolumesArray: {\n\tpvc: *[\n\t\tfor
        v in parameter.volumeMounts.pvc {\n\t\t\t{\n\t\t\t\tname: v.name\n\t\t\t\tpersistentVolumeClaim:
        claimName: v.claimName\n\t\t\t}\n\t\t},\n\t] | []\n\n\tconfigMap: *[\n\t\t\tfor
        v in parameter.volumeMounts.configMap {\n\t\t\t{\n\t\t\t\tname: v.name\n\t\t\t\tconfigMap:
        {\n\t\t\t\t\tdefaultMode: v.defaultMode\n\t\t\t\t\tname:        v.cmName\n\t\t\t\t\tif
        v.items != _|_ {\n\t\t\t\t\t\titems: v.items\n\t\t\t\t\t}\n\t\t\t\t}\n\t\t\t}\n\t\t},\n\t]
        | []\n\n\tsecret: *[\n\t\tfor v in parameter.volumeMounts.secret {\n\t\t\t{\n\t\t\t\tname:
        v.name\n\t\t\t\tsecret: {\n\t\t\t\t\tdefaultMode: v.defaultMode\n\t\t\t\t\tsecretName:
        \ v.secretName\n\t\t\t\t\tif v.items != _|_ {\n\t\t\t\t\t\titems: v.items\n\t\t\t\t\t}\n\t\t\t\t}\n\t\t\t}\n\t\t},\n\t]
        | []\n\n\temptyDir: *[\n\t\t\tfor v in parameter.volumeMounts.emptyDir {\n\t\t\t{\n\t\t\t\tname:
        v.name\n\t\t\t\temptyDir: medium: v.medium\n\t\t\t}\n\t\t},\n\t] | []\n\n\thostPath:
        *[\n\t\t\tfor v in parameter.volumeMounts.hostPath {\n\t\t\t{\n\t\t\t\tname:
        v.name\n\t\t\t\thostPath: path: v.path\n\t\t\t}\n\t\t},\n\t] | []\n}\nvolumesList:
        volumesArray.pvc + volumesArray.configMap + volumesArray.secret + volumesArray.emptyDir
        + volumesArray.hostPath\ndeDupVolumesArray: [\n\tfor val in [\n\t\tfor i,
        vi in volumesList {\n\t\t\tfor j, vj in volumesList if j < i && vi.name ==
        vj.name {\n\t\t\t\t_ignore: true\n\t\t\t}\n\t\t\tvi\n\t\t},\n\t] if val._ignore
        == _|_ {\n\t\tval\n\t},\n]\noutput: {\n\tapiVersion: \"apps/v1\"\n\tkind:
        \      \"Deployment\"\n\tspec: {\n\t\tselector: matchLabels: \"app.oam.dev/component\":
        context.name\n\n\t\ttemplate: {\n\t\t\tmetadata: {\n\t\t\t\tlabels: {\n\t\t\t\t\tif
        parameter.labels != _|_ {\n\t\t\t\t\t\tparameter.labels\n\t\t\t\t\t}\n\t\t\t\t\tif
        parameter.addRevisionLabel {\n\t\t\t\t\t\t\"app.oam.dev/revision\": context.revision\n\t\t\t\t\t}\n\t\t\t\t\t\"app.oam.dev/name\":
        \     context.appName\n\t\t\t\t\t\"app.oam.dev/component\": context.name\n\t\t\t\t}\n\t\t\t\tif
        parameter.annotations != _|_ {\n\t\t\t\t\tannotations: parameter.annotations\n\t\t\t\t}\n\t\t\t}\n\n\t\t\tspec:
        {\n\t\t\t\tcontainers: [{\n\t\t\t\t\tname:  context.name\n\t\t\t\t\timage:
        parameter.image\n\t\t\t\t\tif parameter[\"port\"] != _|_ && parameter[\"ports\"]
        == _|_ {\n\t\t\t\t\t\tports: [{\n\t\t\t\t\t\t\tcontainerPort: parameter.port\n\t\t\t\t\t\t}]\n\t\t\t\t\t}\n\t\t\t\t\tif
        parameter[\"ports\"] != _|_ {\n\t\t\t\t\t\tports: [ for v in parameter.ports
        {\n\t\t\t\t\t\t\t{\n\t\t\t\t\t\t\t\tcontainerPort: v.port\n\t\t\t\t\t\t\t\tprotocol:
        \     v.protocol\n\t\t\t\t\t\t\t\tif v.name != _|_ {\n\t\t\t\t\t\t\t\t\tname:
        v.name\n\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t\tif v.name == _|_ {\n\t\t\t\t\t\t\t\t\tname:
        \"port-\" + strconv.FormatInt(v.port, 10)\n\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t}}]\n\t\t\t\t\t}\n\n\t\t\t\t\tif
        parameter[\"imagePullPolicy\"] != _|_ {\n\t\t\t\t\t\timagePullPolicy: parameter.imagePullPolicy\n\t\t\t\t\t}\n\n\t\t\t\t\tif
        parameter[\"cmd\"] != _|_ {\n\t\t\t\t\t\tcommand: parameter.cmd\n\t\t\t\t\t}\n\n\t\t\t\t\tif
        parameter[\"env\"] != _|_ {\n\t\t\t\t\t\tenv: parameter.env\n\t\t\t\t\t}\n\n\t\t\t\t\tif
        context[\"config\"] != _|_ {\n\t\t\t\t\t\tenv: context.config\n\t\t\t\t\t}\n\n\t\t\t\t\tif
        parameter[\"cpu\"] != _|_ {\n\t\t\t\t\t\tresources: {\n\t\t\t\t\t\t\tlimits:
        cpu:   parameter.cpu\n\t\t\t\t\t\t\trequests: cpu: parameter.cpu\n\t\t\t\t\t\t}\n\t\t\t\t\t}\n\n\t\t\t\t\tif
        parameter[\"memory\"] != _|_ {\n\t\t\t\t\t\tresources: {\n\t\t\t\t\t\t\tlimits:
        memory:   parameter.memory\n\t\t\t\t\t\t\trequests: memory: parameter.memory\n\t\t\t\t\t\t}\n\t\t\t\t\t}\n\n\t\t\t\t\tif
        parameter[\"volumes\"] != _|_ && parameter[\"volumeMounts\"] == _|_ {\n\t\t\t\t\t\tvolumeMounts:
        [ for v in parameter.volumes {\n\t\t\t\t\t\t\t{\n\t\t\t\t\t\t\t\tmountPath:
        v.mountPath\n\t\t\t\t\t\t\t\tname:      v.name\n\t\t\t\t\t\t\t}}]\n\t\t\t\t\t}\n\n\t\t\t\t\tif
        parameter[\"volumeMounts\"] != _|_ {\n\t\t\t\t\t\tvolumeMounts: mountsArray.pvc
        + mountsArray.configMap + mountsArray.secret + mountsArray.emptyDir + mountsArray.hostPath\n\t\t\t\t\t}\n\n\t\t\t\t\tif
        parameter[\"livenessProbe\"] != _|_ {\n\t\t\t\t\t\tlivenessProbe: parameter.livenessProbe\n\t\t\t\t\t}\n\n\t\t\t\t\tif
        parameter[\"readinessProbe\"] != _|_ {\n\t\t\t\t\t\treadinessProbe: parameter.readinessProbe\n\t\t\t\t\t}\n\n\t\t\t\t}]\n\n\t\t\t\tif
        parameter[\"hostAliases\"] != _|_ {\n\t\t\t\t\t// +patchKey=ip\n\t\t\t\t\thostAliases:
        parameter.hostAliases\n\t\t\t\t}\n\n\t\t\t\tif parameter[\"imagePullSecrets\"]
        != _|_ {\n\t\t\t\t\timagePullSecrets: [ for v in parameter.imagePullSecrets
        {\n\t\t\t\t\t\tname: v\n\t\t\t\t\t},\n\t\t\t\t\t]\n\t\t\t\t}\n\n\t\t\t\tif
        parameter[\"volumes\"] != _|_ && parameter[\"volumeMounts\"] == _|_ {\n\t\t\t\t\tvolumes:
        [ for v in parameter.volumes {\n\t\t\t\t\t\t{\n\t\t\t\t\t\t\tname: v.name\n\t\t\t\t\t\t\tif
        v.type == \"pvc\" {\n\t\t\t\t\t\t\t\tpersistentVolumeClaim: claimName: v.claimName\n\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\tif
        v.type == \"configMap\" {\n\t\t\t\t\t\t\t\tconfigMap: {\n\t\t\t\t\t\t\t\t\tdefaultMode:
        v.defaultMode\n\t\t\t\t\t\t\t\t\tname:        v.cmName\n\t\t\t\t\t\t\t\t\tif
        v.items != _|_ {\n\t\t\t\t\t\t\t\t\t\titems: v.items\n\t\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\tif
        v.type == \"secret\" {\n\t\t\t\t\t\t\t\tsecret: {\n\t\t\t\t\t\t\t\t\tdefaultMode:
        v.defaultMode\n\t\t\t\t\t\t\t\t\tsecretName:  v.secretName\n\t\t\t\t\t\t\t\t\tif
        v.items != _|_ {\n\t\t\t\t\t\t\t\t\t\titems: v.items\n\t\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\tif
        v.type == \"emptyDir\" {\n\t\t\t\t\t\t\t\temptyDir: medium: v.medium\n\t\t\t\t\t\t\t}\n\t\t\t\t\t\t}\n\t\t\t\t\t}]\n\t\t\t\t}\n\n\t\t\t\tif
        parameter[\"volumeMounts\"] != _|_ {\n\t\t\t\t\tvolumes: deDupVolumesArray\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n}\nexposePorts:
        [\n\tfor v in parameter.ports if v.expose == true {\n\t\tport:       v.port\n\t\ttargetPort:
        v.port\n\t\tif v.name != _|_ {\n\t\t\tname: v.name\n\t\t}\n\t\tif v.name ==
        _|_ {\n\t\t\tname: \"port-\" + strconv.FormatInt(v.port, 10)\n\t\t}\n\t},\n]\noutputs:
        {\n\tif len(exposePorts) != 0 {\n\t\twebserviceExpose: {\n\t\t\tapiVersion:
        \"v1\"\n\t\t\tkind:       \"Service\"\n\t\t\tmetadata: name: context.name\n\t\t\tspec:
        {\n\t\t\t\tselector: \"app.oam.dev/component\": context.name\n\t\t\t\tports:
        exposePorts\n\t\t\t\ttype:  parameter.exposeType\n\t\t\t}\n\t\t}\n\t}\n}\nparameter:
        {\n\t// +usage=Specify the labels in the workload\n\tlabels?: [string]: string\n\n\t//
        +usage=Specify the annotations in the workload\n\tannotations?: [string]:
        string\n\n\t// +usage=Which image would you like to use for your service\n\t//
        +short=i\n\timage: string\n\n\t// +usage=Specify image pull policy for your
        service\n\timagePullPolicy?: \"Always\" | \"Never\" | \"IfNotPresent\"\n\n\t//
        +usage=Specify image pull secrets for your service\n\timagePullSecrets?: [...string]\n\n\t//
        +ignore\n\t// +usage=Deprecated field, please use ports instead\n\t// +short=p\n\tport?:
        int\n\n\t// +usage=Which ports do you want customer traffic sent to, defaults
        to 80\n\tports?: [...{\n\t\t// +usage=Number of port to expose on the pod's
        IP address\n\t\tport: int\n\t\t// +usage=Name of the port\n\t\tname?: string\n\t\t//
        +usage=Protocol for port. Must be UDP, TCP, or SCTP\n\t\tprotocol: *\"TCP\"
        | \"UDP\" | \"SCTP\"\n\t\t// +usage=Specify if the port should be exposed\n\t\texpose:
        *false | bool\n\t}]\n\n\t// +ignore\n\t// +usage=Specify what kind of Service
        you want. options: \"ClusterIP\", \"NodePort\", \"LoadBalancer\"\n\texposeType:
        *\"ClusterIP\" | \"NodePort\" | \"LoadBalancer\"\n\n\t// +ignore\n\t// +usage=If
        addRevisionLabel is true, the revision label will be added to the underlying
        pods\n\taddRevisionLabel: *false | bool\n\n\t// +usage=Commands to run in
        the container\n\tcmd?: [...string]\n\n\t// +usage=Define arguments by using
        environment variables\n\tenv?: [...{\n\t\t// +usage=Environment variable name\n\t\tname:
        string\n\t\t// +usage=The value of the environment variable\n\t\tvalue?: string\n\t\t//
        +usage=Specifies a source the value of this var should come from\n\t\tvalueFrom?:
        {\n\t\t\t// +usage=Selects a key of a secret in the pod's namespace\n\t\t\tsecretKeyRef?:
        {\n\t\t\t\t// +usage=The name of the secret in the pod's namespace to select
        from\n\t\t\t\tname: string\n\t\t\t\t// +usage=The key of the secret to select
        from. Must be a valid secret key\n\t\t\t\tkey: string\n\t\t\t}\n\t\t\t// +usage=Selects
        a key of a config map in the pod's namespace\n\t\t\tconfigMapKeyRef?: {\n\t\t\t\t//
        +usage=The name of the config map in the pod's namespace to select from\n\t\t\t\tname:
        string\n\t\t\t\t// +usage=The key of the config map to select from. Must be
        a valid secret key\n\t\t\t\tkey: string\n\t\t\t}\n\t\t}\n\t}]\n\n\t// +usage=Number
        of CPU units for the service\n\tcpu?: string\n\n\t// +usage=Specifies the
        attributes of the memory resource required for the container.\n\tmemory?:
        string\n\n\tvolumeMounts?: {\n\t\t// +usage=Mount PVC type volume\n\t\tpvc?:
        [...{\n\t\t\tname:      string\n\t\t\tmountPath: string\n\t\t\tsubPath?:  string\n\t\t\t//
        +usage=The name of the PVC\n\t\t\tclaimName: string\n\t\t}]\n\t\t// +usage=Mount
        ConfigMap type volume\n\t\tconfigMap?: [...{\n\t\t\tname:        string\n\t\t\tmountPath:
        \  string\n\t\t\tsubPath?:    string\n\t\t\tdefaultMode: *420 | int\n\t\t\tcmName:
        \     string\n\t\t\titems?: [...{\n\t\t\t\tkey:  string\n\t\t\t\tpath: string\n\t\t\t\tmode:
        *511 | int\n\t\t\t}]\n\t\t}]\n\t\t// +usage=Mount Secret type volume\n\t\tsecret?:
        [...{\n\t\t\tname:        string\n\t\t\tmountPath:   string\n\t\t\tsubPath?:
        \   string\n\t\t\tdefaultMode: *420 | int\n\t\t\tsecretName:  string\n\t\t\titems?:
        [...{\n\t\t\t\tkey:  string\n\t\t\t\tpath: string\n\t\t\t\tmode: *511 | int\n\t\t\t}]\n\t\t}]\n\t\t//
        +usage=Mount EmptyDir type volume\n\t\temptyDir?: [...{\n\t\t\tname:      string\n\t\t\tmountPath:
        string\n\t\t\tsubPath?:  string\n\t\t\tmedium:    *\"\" | \"Memory\"\n\t\t}]\n\t\t//
        +usage=Mount HostPath type volume\n\t\thostPath?: [...{\n\t\t\tname:      string\n\t\t\tmountPath:
        string\n\t\t\tsubPath?:  string\n\t\t\tpath:      string\n\t\t}]\n\t}\n\n\t//
        +usage=Deprecated field, use volumeMounts instead.\n\tvolumes?: [...{\n\t\tname:
        \     string\n\t\tmountPath: string\n\t\t// +usage=Specify volume type, options:
        \"pvc\",\"configMap\",\"secret\",\"emptyDir\"\n\t\ttype: \"pvc\" | \"configMap\"
        | \"secret\" | \"emptyDir\"\n\t\tif type == \"pvc\" {\n\t\t\tclaimName: string\n\t\t}\n\t\tif
        type == \"configMap\" {\n\t\t\tdefaultMode: *420 | int\n\t\t\tcmName:      string\n\t\t\titems?:
        [...{\n\t\t\t\tkey:  string\n\t\t\t\tpath: string\n\t\t\t\tmode: *511 | int\n\t\t\t}]\n\t\t}\n\t\tif
        type == \"secret\" {\n\t\t\tdefaultMode: *420 | int\n\t\t\tsecretName:  string\n\t\t\titems?:
        [...{\n\t\t\t\tkey:  string\n\t\t\t\tpath: string\n\t\t\t\tmode: *511 | int\n\t\t\t}]\n\t\t}\n\t\tif
        type == \"emptyDir\" {\n\t\t\tmedium: *\"\" | \"Memory\"\n\t\t}\n\t}]\n\n\t//
        +usage=Instructions for assessing whether the container is alive.\n\tlivenessProbe?:
        #HealthProbe\n\n\t// +usage=Instructions for assessing whether the container
        is in a suitable state to serve traffic.\n\treadinessProbe?: #HealthProbe\n\n\t//
        +usage=Specify the hostAliases to add\n\thostAliases?: [...{\n\t\tip: string\n\t\thostnames:
        [...string]\n\t}]\n}\n#HealthProbe: {\n\n\t// +usage=Instructions for assessing
        container health by executing a command. Either this attribute or the httpGet
        attribute or the tcpSocket attribute MUST be specified. This attribute is
        mutually exclusive with both the httpGet attribute and the tcpSocket attribute.\n\texec?:
        {\n\t\t// +usage=A command to be executed inside the container to assess its
        health. Each space delimited token of the command is a separate array element.
        Commands exiting 0 are considered to be successful probes, whilst all other
        exit codes are considered failures.\n\t\tcommand: [...string]\n\t}\n\n\t//
        +usage=Instructions for assessing container health by executing an HTTP GET
        request. Either this attribute or the exec attribute or the tcpSocket attribute
        MUST be specified. This attribute is mutually exclusive with both the exec
        attribute and the tcpSocket attribute.\n\thttpGet?: {\n\t\t// +usage=The endpoint,
        relative to the port, to which the HTTP GET request should be directed.\n\t\tpath:
        string\n\t\t// +usage=The TCP socket within the container to which the HTTP
        GET request should be directed.\n\t\tport:    int\n\t\thost?:   string\n\t\tscheme?:
        *\"HTTP\" | string\n\t\thttpHeaders?: [...{\n\t\t\tname:  string\n\t\t\tvalue:
        string\n\t\t}]\n\t}\n\n\t// +usage=Instructions for assessing container health
        by probing a TCP socket. Either this attribute or the exec attribute or the
        httpGet attribute MUST be specified. This attribute is mutually exclusive
        with both the exec attribute and the httpGet attribute.\n\ttcpSocket?: {\n\t\t//
        +usage=The TCP socket within the container that should be probed to assess
        container health.\n\t\tport: int\n\t}\n\n\t// +usage=Number of seconds after
        the container is started before the first probe is initiated.\n\tinitialDelaySeconds:
        *0 | int\n\n\t// +usage=How often, in seconds, to execute the probe.\n\tperiodSeconds:
        *10 | int\n\n\t// +usage=Number of seconds after which the probe times out.\n\ttimeoutSeconds:
        *1 | int\n\n\t// +usage=Minimum consecutive successes for the probe to be
        considered successful after having failed.\n\tsuccessThreshold: *1 | int\n\n\t//
        +usage=Number of consecutive failures required to determine the container
        is not alive (liveness probe) or not ready (readiness probe).\n\tfailureThreshold:
        *3 | int\n}\n"
  status:
    customStatus: "ready: {\n\treadyReplicas: *0 | int\n} & {\n\tif context.output.status.readyReplicas
      != _|_ {\n\t\treadyReplicas: context.output.status.readyReplicas\n\t}\n}\nmessage:
      \"Ready:\\(ready.readyReplicas)/\\(context.output.spec.replicas)\""
    healthPolicy: "ready: {\n\tupdatedReplicas:    *0 | int\n\treadyReplicas:      *0
      | int\n\treplicas:           *0 | int\n\tobservedGeneration: *0 | int\n} & {\n\tif
      context.output.status.updatedReplicas != _|_ {\n\t\tupdatedReplicas: context.output.status.updatedReplicas\n\t}\n\tif
      context.output.status.readyReplicas != _|_ {\n\t\treadyReplicas: context.output.status.readyReplicas\n\t}\n\tif
      context.output.status.replicas != _|_ {\n\t\treplicas: context.output.status.replicas\n\t}\n\tif
      context.output.status.observedGeneration != _|_ {\n\t\tobservedGeneration: context.output.status.observedGeneration\n\t}\n}\nisHealth:
      (context.output.spec.replicas == ready.readyReplicas) && (context.output.spec.replicas
      == ready.updatedReplicas) && (context.output.spec.replicas == ready.replicas)
      && (ready.observedGeneration == context.output.metadata.generation || ready.observedGeneration
      > context.output.metadata.generation)"
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
    type: deployments.apps
status: {}
`

var firstVelaAppRev string = `
apiVersion: core.oam.dev/v1beta1
kind: ApplicationRevision
metadata:
  annotations:
    oam.dev/kubevela-version: v1.5.2
  generation: 1
  labels:
    app.oam.dev/app-revision-hash: 1c3d847600ac0514
    app.oam.dev/name: first-vela-app
  name: first-vela-app-v1
  namespace: vela-system
spec:
  application:
    apiVersion: core.oam.dev/v1beta1
    kind: Application
    metadata:
      annotations:
      finalizers:
      - app.oam.dev/resource-tracker-finalizer
      name: first-vela-app
      namespace: vela-system
    spec:
      components:
      - name: express-server
        properties:
          image: oamdev/hello-world
          ports:
          - expose: true
            port: 8000
        traits:
        - properties:
            replicas: 1
          type: scaler
        type: webservice
    status: {}
  componentDefinitions:
    webservice:
      apiVersion: core.oam.dev/v1beta1
      kind: ComponentDefinition
      metadata:
        annotations:
          definition.oam.dev/description: Describes long-running, scalable, containerized
            services that have a stable network endpoint to receive external network
            traffic from customers.
          meta.helm.sh/release-name: kubevela
          meta.helm.sh/release-namespace: vela-system
        labels:
          app.kubernetes.io/managed-by: Helm
        name: webservice
        namespace: vela-system
      spec:
        schematic:
          cue:
            template: "import (\n\t\"strconv\"\n)\n\nmountsArray: {\n\tpvc: *[\n\t\tfor
              v in parameter.volumeMounts.pvc {\n\t\t\t{\n\t\t\t\tmountPath: v.mountPath\n\t\t\t\tif
              v.subPath != _|_ {\n\t\t\t\t\tsubPath: v.subPath\n\t\t\t\t}\n\t\t\t\tname:
              v.name\n\t\t\t}\n\t\t},\n\t] | []\n\n\tconfigMap: *[\n\t\t\tfor v in
              parameter.volumeMounts.configMap {\n\t\t\t{\n\t\t\t\tmountPath: v.mountPath\n\t\t\t\tif
              v.subPath != _|_ {\n\t\t\t\t\tsubPath: v.subPath\n\t\t\t\t}\n\t\t\t\tname:
              v.name\n\t\t\t}\n\t\t},\n\t] | []\n\n\tsecret: *[\n\t\tfor v in parameter.volumeMounts.secret
              {\n\t\t\t{\n\t\t\t\tmountPath: v.mountPath\n\t\t\t\tif v.subPath !=
              _|_ {\n\t\t\t\t\tsubPath: v.subPath\n\t\t\t\t}\n\t\t\t\tname: v.name\n\t\t\t}\n\t\t},\n\t]
              | []\n\n\temptyDir: *[\n\t\t\tfor v in parameter.volumeMounts.emptyDir
              {\n\t\t\t{\n\t\t\t\tmountPath: v.mountPath\n\t\t\t\tif v.subPath !=
              _|_ {\n\t\t\t\t\tsubPath: v.subPath\n\t\t\t\t}\n\t\t\t\tname: v.name\n\t\t\t}\n\t\t},\n\t]
              | []\n\n\thostPath: *[\n\t\t\tfor v in parameter.volumeMounts.hostPath
              {\n\t\t\t{\n\t\t\t\tmountPath: v.mountPath\n\t\t\t\tif v.subPath !=
              _|_ {\n\t\t\t\t\tsubPath: v.subPath\n\t\t\t\t}\n\t\t\t\tname: v.name\n\t\t\t}\n\t\t},\n\t]
              | []\n}\nvolumesArray: {\n\tpvc: *[\n\t\tfor v in parameter.volumeMounts.pvc
              {\n\t\t\t{\n\t\t\t\tname: v.name\n\t\t\t\tpersistentVolumeClaim: claimName:
              v.claimName\n\t\t\t}\n\t\t},\n\t] | []\n\n\tconfigMap: *[\n\t\t\tfor
              v in parameter.volumeMounts.configMap {\n\t\t\t{\n\t\t\t\tname: v.name\n\t\t\t\tconfigMap:
              {\n\t\t\t\t\tdefaultMode: v.defaultMode\n\t\t\t\t\tname:        v.cmName\n\t\t\t\t\tif
              v.items != _|_ {\n\t\t\t\t\t\titems: v.items\n\t\t\t\t\t}\n\t\t\t\t}\n\t\t\t}\n\t\t},\n\t]
              | []\n\n\tsecret: *[\n\t\tfor v in parameter.volumeMounts.secret {\n\t\t\t{\n\t\t\t\tname:
              v.name\n\t\t\t\tsecret: {\n\t\t\t\t\tdefaultMode: v.defaultMode\n\t\t\t\t\tsecretName:
              \ v.secretName\n\t\t\t\t\tif v.items != _|_ {\n\t\t\t\t\t\titems: v.items\n\t\t\t\t\t}\n\t\t\t\t}\n\t\t\t}\n\t\t},\n\t]
              | []\n\n\temptyDir: *[\n\t\t\tfor v in parameter.volumeMounts.emptyDir
              {\n\t\t\t{\n\t\t\t\tname: v.name\n\t\t\t\temptyDir: medium: v.medium\n\t\t\t}\n\t\t},\n\t]
              | []\n\n\thostPath: *[\n\t\t\tfor v in parameter.volumeMounts.hostPath
              {\n\t\t\t{\n\t\t\t\tname: v.name\n\t\t\t\thostPath: path: v.path\n\t\t\t}\n\t\t},\n\t]
              | []\n}\nvolumesList: volumesArray.pvc + volumesArray.configMap + volumesArray.secret
              + volumesArray.emptyDir + volumesArray.hostPath\ndeDupVolumesArray:
              [\n\tfor val in [\n\t\tfor i, vi in volumesList {\n\t\t\tfor j, vj in
              volumesList if j < i && vi.name == vj.name {\n\t\t\t\t_ignore: true\n\t\t\t}\n\t\t\tvi\n\t\t},\n\t]
              if val._ignore == _|_ {\n\t\tval\n\t},\n]\noutput: {\n\tapiVersion:
              \"apps/v1\"\n\tkind:       \"Deployment\"\n\tspec: {\n\t\tselector:
              matchLabels: \"app.oam.dev/component\": context.name\n\n\t\ttemplate:
              {\n\t\t\tmetadata: {\n\t\t\t\tlabels: {\n\t\t\t\t\tif parameter.labels
              != _|_ {\n\t\t\t\t\t\tparameter.labels\n\t\t\t\t\t}\n\t\t\t\t\tif parameter.addRevisionLabel
              {\n\t\t\t\t\t\t\"app.oam.dev/revision\": context.revision\n\t\t\t\t\t}\n\t\t\t\t\t\"app.oam.dev/name\":
              \     context.appName\n\t\t\t\t\t\"app.oam.dev/component\": context.name\n\t\t\t\t}\n\t\t\t\tif
              parameter.annotations != _|_ {\n\t\t\t\t\tannotations: parameter.annotations\n\t\t\t\t}\n\t\t\t}\n\n\t\t\tspec:
              {\n\t\t\t\tcontainers: [{\n\t\t\t\t\tname:  context.name\n\t\t\t\t\timage:
              parameter.image\n\t\t\t\t\tif parameter[\"port\"] != _|_ && parameter[\"ports\"]
              == _|_ {\n\t\t\t\t\t\tports: [{\n\t\t\t\t\t\t\tcontainerPort: parameter.port\n\t\t\t\t\t\t}]\n\t\t\t\t\t}\n\t\t\t\t\tif
              parameter[\"ports\"] != _|_ {\n\t\t\t\t\t\tports: [ for v in parameter.ports
              {\n\t\t\t\t\t\t\t{\n\t\t\t\t\t\t\t\tcontainerPort: v.port\n\t\t\t\t\t\t\t\tprotocol:
              \     v.protocol\n\t\t\t\t\t\t\t\tif v.name != _|_ {\n\t\t\t\t\t\t\t\t\tname:
              v.name\n\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t\tif v.name == _|_ {\n\t\t\t\t\t\t\t\t\tname:
              \"port-\" + strconv.FormatInt(v.port, 10)\n\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t}}]\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              parameter[\"imagePullPolicy\"] != _|_ {\n\t\t\t\t\t\timagePullPolicy:
              parameter.imagePullPolicy\n\t\t\t\t\t}\n\n\t\t\t\t\tif parameter[\"cmd\"]
              != _|_ {\n\t\t\t\t\t\tcommand: parameter.cmd\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              parameter[\"env\"] != _|_ {\n\t\t\t\t\t\tenv: parameter.env\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              context[\"config\"] != _|_ {\n\t\t\t\t\t\tenv: context.config\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              parameter[\"cpu\"] != _|_ {\n\t\t\t\t\t\tresources: {\n\t\t\t\t\t\t\tlimits:
              cpu:   parameter.cpu\n\t\t\t\t\t\t\trequests: cpu: parameter.cpu\n\t\t\t\t\t\t}\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              parameter[\"memory\"] != _|_ {\n\t\t\t\t\t\tresources: {\n\t\t\t\t\t\t\tlimits:
              memory:   parameter.memory\n\t\t\t\t\t\t\trequests: memory: parameter.memory\n\t\t\t\t\t\t}\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              parameter[\"volumes\"] != _|_ && parameter[\"volumeMounts\"] == _|_
              {\n\t\t\t\t\t\tvolumeMounts: [ for v in parameter.volumes {\n\t\t\t\t\t\t\t{\n\t\t\t\t\t\t\t\tmountPath:
              v.mountPath\n\t\t\t\t\t\t\t\tname:      v.name\n\t\t\t\t\t\t\t}}]\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              parameter[\"volumeMounts\"] != _|_ {\n\t\t\t\t\t\tvolumeMounts: mountsArray.pvc
              + mountsArray.configMap + mountsArray.secret + mountsArray.emptyDir
              + mountsArray.hostPath\n\t\t\t\t\t}\n\n\t\t\t\t\tif parameter[\"livenessProbe\"]
              != _|_ {\n\t\t\t\t\t\tlivenessProbe: parameter.livenessProbe\n\t\t\t\t\t}\n\n\t\t\t\t\tif
              parameter[\"readinessProbe\"] != _|_ {\n\t\t\t\t\t\treadinessProbe:
              parameter.readinessProbe\n\t\t\t\t\t}\n\n\t\t\t\t}]\n\n\t\t\t\tif parameter[\"hostAliases\"]
              != _|_ {\n\t\t\t\t\t// +patchKey=ip\n\t\t\t\t\thostAliases: parameter.hostAliases\n\t\t\t\t}\n\n\t\t\t\tif
              parameter[\"imagePullSecrets\"] != _|_ {\n\t\t\t\t\timagePullSecrets:
              [ for v in parameter.imagePullSecrets {\n\t\t\t\t\t\tname: v\n\t\t\t\t\t},\n\t\t\t\t\t]\n\t\t\t\t}\n\n\t\t\t\tif
              parameter[\"volumes\"] != _|_ && parameter[\"volumeMounts\"] == _|_
              {\n\t\t\t\t\tvolumes: [ for v in parameter.volumes {\n\t\t\t\t\t\t{\n\t\t\t\t\t\t\tname:
              v.name\n\t\t\t\t\t\t\tif v.type == \"pvc\" {\n\t\t\t\t\t\t\t\tpersistentVolumeClaim:
              claimName: v.claimName\n\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\tif v.type ==
              \"configMap\" {\n\t\t\t\t\t\t\t\tconfigMap: {\n\t\t\t\t\t\t\t\t\tdefaultMode:
              v.defaultMode\n\t\t\t\t\t\t\t\t\tname:        v.cmName\n\t\t\t\t\t\t\t\t\tif
              v.items != _|_ {\n\t\t\t\t\t\t\t\t\t\titems: v.items\n\t\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\tif
              v.type == \"secret\" {\n\t\t\t\t\t\t\t\tsecret: {\n\t\t\t\t\t\t\t\t\tdefaultMode:
              v.defaultMode\n\t\t\t\t\t\t\t\t\tsecretName:  v.secretName\n\t\t\t\t\t\t\t\t\tif
              v.items != _|_ {\n\t\t\t\t\t\t\t\t\t\titems: v.items\n\t\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\tif
              v.type == \"emptyDir\" {\n\t\t\t\t\t\t\t\temptyDir: medium: v.medium\n\t\t\t\t\t\t\t}\n\t\t\t\t\t\t}\n\t\t\t\t\t}]\n\t\t\t\t}\n\n\t\t\t\tif
              parameter[\"volumeMounts\"] != _|_ {\n\t\t\t\t\tvolumes: deDupVolumesArray\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n}\nexposePorts:
              [\n\tfor v in parameter.ports if v.expose == true {\n\t\tport:       v.port\n\t\ttargetPort:
              v.port\n\t\tif v.name != _|_ {\n\t\t\tname: v.name\n\t\t}\n\t\tif v.name
              == _|_ {\n\t\t\tname: \"port-\" + strconv.FormatInt(v.port, 10)\n\t\t}\n\t},\n]\noutputs:
              {\n\tif len(exposePorts) != 0 {\n\t\twebserviceExpose: {\n\t\t\tapiVersion:
              \"v1\"\n\t\t\tkind:       \"Service\"\n\t\t\tmetadata: name: context.name\n\t\t\tspec:
              {\n\t\t\t\tselector: \"app.oam.dev/component\": context.name\n\t\t\t\tports:
              exposePorts\n\t\t\t\ttype:  parameter.exposeType\n\t\t\t}\n\t\t}\n\t}\n}\nparameter:
              {\n\t// +usage=Specify the labels in the workload\n\tlabels?: [string]:
              string\n\n\t// +usage=Specify the annotations in the workload\n\tannotations?:
              [string]: string\n\n\t// +usage=Which image would you like to use for
              your service\n\t// +short=i\n\timage: string\n\n\t// +usage=Specify
              image pull policy for your service\n\timagePullPolicy?: \"Always\" |
              \"Never\" | \"IfNotPresent\"\n\n\t// +usage=Specify image pull secrets
              for your service\n\timagePullSecrets?: [...string]\n\n\t// +ignore\n\t//
              +usage=Deprecated field, please use ports instead\n\t// +short=p\n\tport?:
              int\n\n\t// +usage=Which ports do you want customer traffic sent to,
              defaults to 80\n\tports?: [...{\n\t\t// +usage=Number of port to expose
              on the pod's IP address\n\t\tport: int\n\t\t// +usage=Name of the port\n\t\tname?:
              string\n\t\t// +usage=Protocol for port. Must be UDP, TCP, or SCTP\n\t\tprotocol:
              *\"TCP\" | \"UDP\" | \"SCTP\"\n\t\t// +usage=Specify if the port should
              be exposed\n\t\texpose: *false | bool\n\t}]\n\n\t// +ignore\n\t// +usage=Specify
              what kind of Service you want. options: \"ClusterIP\", \"NodePort\",
              \"LoadBalancer\"\n\texposeType: *\"ClusterIP\" | \"NodePort\" | \"LoadBalancer\"\n\n\t//
              +ignore\n\t// +usage=If addRevisionLabel is true, the revision label
              will be added to the underlying pods\n\taddRevisionLabel: *false | bool\n\n\t//
              +usage=Commands to run in the container\n\tcmd?: [...string]\n\n\t//
              +usage=Define arguments by using environment variables\n\tenv?: [...{\n\t\t//
              +usage=Environment variable name\n\t\tname: string\n\t\t// +usage=The
              value of the environment variable\n\t\tvalue?: string\n\t\t// +usage=Specifies
              a source the value of this var should come from\n\t\tvalueFrom?: {\n\t\t\t//
              +usage=Selects a key of a secret in the pod's namespace\n\t\t\tsecretKeyRef?:
              {\n\t\t\t\t// +usage=The name of the secret in the pod's namespace to
              select from\n\t\t\t\tname: string\n\t\t\t\t// +usage=The key of the
              secret to select from. Must be a valid secret key\n\t\t\t\tkey: string\n\t\t\t}\n\t\t\t//
              +usage=Selects a key of a config map in the pod's namespace\n\t\t\tconfigMapKeyRef?:
              {\n\t\t\t\t// +usage=The name of the config map in the pod's namespace
              to select from\n\t\t\t\tname: string\n\t\t\t\t// +usage=The key of the
              config map to select from. Must be a valid secret key\n\t\t\t\tkey:
              string\n\t\t\t}\n\t\t}\n\t}]\n\n\t// +usage=Number of CPU units for
              the service\n\tcpu?: string\n\n\t//
              +usage=Specifies the attributes of the memory resource required for
              the container.\n\tmemory?: string\n\n\tvolumeMounts?: {\n\t\t// +usage=Mount
              PVC type volume\n\t\tpvc?: [...{\n\t\t\tname:      string\n\t\t\tmountPath:
              string\n\t\t\tsubPath?:  string\n\t\t\t// +usage=The name of the PVC\n\t\t\tclaimName:
              string\n\t\t}]\n\t\t// +usage=Mount ConfigMap type volume\n\t\tconfigMap?:
              [...{\n\t\t\tname:        string\n\t\t\tmountPath:   string\n\t\t\tsubPath?:
              \   string\n\t\t\tdefaultMode: *420 | int\n\t\t\tcmName:      string\n\t\t\titems?:
              [...{\n\t\t\t\tkey:  string\n\t\t\t\tpath: string\n\t\t\t\tmode: *511
              | int\n\t\t\t}]\n\t\t}]\n\t\t// +usage=Mount Secret type volume\n\t\tsecret?:
              [...{\n\t\t\tname:        string\n\t\t\tmountPath:   string\n\t\t\tsubPath?:
              \   string\n\t\t\tdefaultMode: *420 | int\n\t\t\tsecretName:  string\n\t\t\titems?:
              [...{\n\t\t\t\tkey:  string\n\t\t\t\tpath: string\n\t\t\t\tmode: *511
              | int\n\t\t\t}]\n\t\t}]\n\t\t// +usage=Mount EmptyDir type volume\n\t\temptyDir?:
              [...{\n\t\t\tname:      string\n\t\t\tmountPath: string\n\t\t\tsubPath?:
              \ string\n\t\t\tmedium:    *\"\" | \"Memory\"\n\t\t}]\n\t\t// +usage=Mount
              HostPath type volume\n\t\thostPath?: [...{\n\t\t\tname:      string\n\t\t\tmountPath:
              string\n\t\t\tsubPath?:  string\n\t\t\tpath:      string\n\t\t}]\n\t}\n\n\t//
              +usage=Deprecated field, use volumeMounts instead.\n\tvolumes?: [...{\n\t\tname:
              \     string\n\t\tmountPath: string\n\t\t// +usage=Specify volume type,
              options: \"pvc\",\"configMap\",\"secret\",\"emptyDir\"\n\t\ttype: \"pvc\"
              | \"configMap\" | \"secret\" | \"emptyDir\"\n\t\tif type == \"pvc\"
              {\n\t\t\tclaimName: string\n\t\t}\n\t\tif type == \"configMap\" {\n\t\t\tdefaultMode:
              *420 | int\n\t\t\tcmName:      string\n\t\t\titems?: [...{\n\t\t\t\tkey:
              \ string\n\t\t\t\tpath: string\n\t\t\t\tmode: *511 | int\n\t\t\t}]\n\t\t}\n\t\tif
              type == \"secret\" {\n\t\t\tdefaultMode: *420 | int\n\t\t\tsecretName:
              \ string\n\t\t\titems?: [...{\n\t\t\t\tkey:  string\n\t\t\t\tpath: string\n\t\t\t\tmode:
              *511 | int\n\t\t\t}]\n\t\t}\n\t\tif type == \"emptyDir\" {\n\t\t\tmedium:
              *\"\" | \"Memory\"\n\t\t}\n\t}]\n\n\t// +usage=Instructions for assessing
              whether the container is alive.\n\tlivenessProbe?: #HealthProbe\n\n\t//
              +usage=Instructions for assessing whether the container is in a suitable
              state to serve traffic.\n\treadinessProbe?: #HealthProbe\n\n\t// +usage=Specify
              the hostAliases to add\n\thostAliases?: [...{\n\t\tip: string\n\t\thostnames:
              [...string]\n\t}]\n}\n#HealthProbe: {\n\n\t// +usage=Instructions for
              assessing container health by executing a command. Either this attribute
              or the httpGet attribute or the tcpSocket attribute MUST be specified.
              This attribute is mutually exclusive with both the httpGet attribute
              and the tcpSocket attribute.\n\texec?: {\n\t\t// +usage=A command to
              be executed inside the container to assess its health. Each space delimited
              token of the command is a separate array element. Commands exiting 0
              are considered to be successful probes, whilst all other exit codes
              are considered failures.\n\t\tcommand: [...string]\n\t}\n\n\t// +usage=Instructions
              for assessing container health by executing an HTTP GET request. Either
              this attribute or the exec attribute or the tcpSocket attribute MUST
              be specified. This attribute is mutually exclusive with both the exec
              attribute and the tcpSocket attribute.\n\thttpGet?: {\n\t\t// +usage=The
              endpoint, relative to the port, to which the HTTP GET request should
              be directed.\n\t\tpath: string\n\t\t// +usage=The TCP socket within
              the container to which the HTTP GET request should be directed.\n\t\tport:
              \   int\n\t\thost?:   string\n\t\tscheme?: *\"HTTP\" | string\n\t\thttpHeaders?:
              [...{\n\t\t\tname:  string\n\t\t\tvalue: string\n\t\t}]\n\t}\n\n\t//
              +usage=Instructions for assessing container health by probing a TCP
              socket. Either this attribute or the exec attribute or the httpGet attribute
              MUST be specified. This attribute is mutually exclusive with both the
              exec attribute and the httpGet attribute.\n\ttcpSocket?: {\n\t\t// +usage=The
              TCP socket within the container that should be probed to assess container
              health.\n\t\tport: int\n\t}\n\n\t// +usage=Number of seconds after the
              container is started before the first probe is initiated.\n\tinitialDelaySeconds:
              *0 | int\n\n\t// +usage=How often, in seconds, to execute the probe.\n\tperiodSeconds:
              *10 | int\n\n\t// +usage=Number of seconds after which the probe times
              out.\n\ttimeoutSeconds: *1 | int\n\n\t// +usage=Minimum consecutive
              successes for the probe to be considered successful after having failed.\n\tsuccessThreshold:
              *1 | int\n\n\t// +usage=Number of consecutive failures required to determine
              the container is not alive (liveness probe) or not ready (readiness
              probe).\n\tfailureThreshold: *3 | int\n}\n"
        status:
          customStatus: "ready: {\n\treadyReplicas: *0 | int\n} & {\n\tif context.output.status.readyReplicas
            != _|_ {\n\t\treadyReplicas: context.output.status.readyReplicas\n\t}\n}\nmessage:
            \"Ready:\\(ready.readyReplicas)/\\(context.output.spec.replicas)\""
          healthPolicy: "ready: {\n\tupdatedReplicas:    *0 | int\n\treadyReplicas:
            \     *0 | int\n\treplicas:           *0 | int\n\tobservedGeneration:
            *0 | int\n} & {\n\tif context.output.status.updatedReplicas != _|_ {\n\t\tupdatedReplicas:
            context.output.status.updatedReplicas\n\t}\n\tif context.output.status.readyReplicas
            != _|_ {\n\t\treadyReplicas: context.output.status.readyReplicas\n\t}\n\tif
            context.output.status.replicas != _|_ {\n\t\treplicas: context.output.status.replicas\n\t}\n\tif
            context.output.status.observedGeneration != _|_ {\n\t\tobservedGeneration:
            context.output.status.observedGeneration\n\t}\n}\nisHealth: (context.output.spec.replicas
            == ready.readyReplicas) && (context.output.spec.replicas == ready.updatedReplicas)
            && (context.output.spec.replicas == ready.replicas) && (ready.observedGeneration
            == context.output.metadata.generation || ready.observedGeneration > context.output.metadata.generation)"
        workload:
          definition:
            apiVersion: apps/v1
            kind: Deployment
          type: deployments.apps
      status: {}

status: {}
`

var _ = Describe("Test getRevision", func() {

	var (
		ctx       context.Context
		arg       common.Args
		name      string
		namespace string
		format    string
		out       *bytes.Buffer
		def       string
	)

	BeforeEach(func() {
		// delete application and view if exist
		app := v1beta1.ApplicationRevision{}
		Expect(yaml.Unmarshal([]byte(firstVelaAppRev), &app)).Should(BeNil())
		_ = k8sClient.Delete(context.TODO(), &app)
		_ = k8sClient.Delete(context.TODO(), &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      revisionView,
				Namespace: types.DefaultKubeVelaNS,
			}})

		// prepare args
		ctx = context.Background()
		format = ""
		out = &bytes.Buffer{}
		arg = common.Args{}
		arg.SetConfig(cfg)
		arg.SetClient(k8sClient)
		name = "first-vela-app-v1"
		namespace = types.DefaultKubeVelaNS
		def = ""
	})

	It("Test no pre-defined view", func() {
		err := getRevision(ctx, arg, format, out, name, namespace, def)
		Expect(err).ToNot(Succeed())
		Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("Unable to get application revision %s in namespace %s", name, namespace)))
	})

	It("Test with no application revision existing", func() {

		// setup view
		setupView()

		err := getRevision(ctx, arg, format, out, name, namespace, def)
		Expect(err).To(Succeed())
		Expect(out.String()).To(Equal(fmt.Sprintf("No such application revision %s in namespace %s", name, namespace)))
	})

	It("Test normal case with default output", func() {

		// setup view
		setupView()

		// setup application
		app := v1beta1.ApplicationRevision{}
		Expect(yaml.Unmarshal([]byte(firstVelaAppRev), &app)).Should(BeNil())
		Expect(k8sClient.Create(context.TODO(), &app)).Should(BeNil())

		Expect(getRevision(ctx, arg, format, out, name, namespace, def)).To(Succeed())
		table := newUITable().AddRow("NAME", "PUBLISH_VERSION", "SUCCEEDED", "HASH", "BEGIN_TIME", "STATUS", "SIZE")
		table.AddRow("first-vela-app-v1", "", "false", "1c3d847600ac0514", "", "NotStart", "")
		Expect(strings.ReplaceAll(out.String(), " ", "")).To(ContainSubstring(strings.ReplaceAll(table.String(), " ", "")))
	})

	It("Test normal case with yaml format", func() {

		// setup view
		setupView()

		// setup application
		app := v1beta1.ApplicationRevision{}
		Expect(yaml.Unmarshal([]byte(firstVelaAppRev), &app)).Should(BeNil())
		Expect(k8sClient.Create(context.TODO(), &app)).Should(BeNil())

		// override args
		format = "yaml"

		Expect(getRevision(ctx, arg, format, out, name, namespace, def)).To(Succeed())
		Expect(out.String()).Should(SatisfyAll(
			ContainSubstring("app.oam.dev/name: first-vela-app"),
			ContainSubstring("name: first-vela-app-v1"),
			ContainSubstring("- name: express-server"),
			ContainSubstring("succeeded: false"),
		))
	})

	It("Test normal case with returning definition", func() {

		// setup view
		setupView()

		// setup application
		app := v1beta1.ApplicationRevision{}
		Expect(yaml.Unmarshal([]byte(firstVelaAppRev), &app)).Should(BeNil())
		Expect(k8sClient.Create(context.TODO(), &app)).Should(BeNil())

		// override args
		def = "webservice"

		Expect(getRevision(ctx, arg, format, out, name, namespace, def)).To(Succeed())
		Expect(out.String()).Should(Equal(compDef))
	})

	It("Test normal case with returning unknown definition", func() {

		// setup view
		setupView()

		// setup application
		app := v1beta1.ApplicationRevision{}
		Expect(yaml.Unmarshal([]byte(firstVelaAppRev), &app)).Should(BeNil())
		Expect(k8sClient.Create(context.TODO(), &app)).Should(BeNil())

		// prepare args
		def = "webservice1"

		Expect(getRevision(ctx, arg, format, out, name, namespace, def)).To(Succeed())
		Expect(out.String()).Should(Equal(fmt.Sprintf("No such definition %s", def)))
	})
})

func TestPrintApprev(t *testing.T) {

	tiFormat := "2006-01-02T15:04:05.000Z"
	tiStr := "2022-08-12T11:45:26.371Z"
	ti, err := time.Parse(tiFormat, tiStr)
	assert.Nil(t, err)

	cases := map[string]struct {
		out    *bytes.Buffer
		apprev v1beta1.ApplicationRevision
		exp    string
	}{
		"NotStart": {out: &bytes.Buffer{}, apprev: v1beta1.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-apprev0",
				Namespace: "dev",
			},
			Spec:   v1beta1.ApplicationRevisionSpec{},
			Status: v1beta1.ApplicationRevisionStatus{},
		}, exp: tableOut("test-apprev0", "", "false", "", "", "NotStart"),
		},
		"Succeeded": {out: &bytes.Buffer{}, apprev: v1beta1.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-apprev1",
				Namespace: "dev1",
				Labels: map[string]string{
					oam.LabelAppRevisionHash: "1111231adfdf",
				},
			},
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					Application: v1beta1.Application{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-app1",
							Namespace: "dev2",
						},
					},
				},
			},
			Status: v1beta1.ApplicationRevisionStatus{
				Workflow: &common2.WorkflowStatus{
					StartTime: metav1.Time{
						Time: ti,
					},
				},
				Succeeded: true,
			},
		}, exp: tableOut("test-apprev1", "", "true", "1111231adfdf", "2022-08-12 11:45:26", "Succeeded")},
		"Failed": {out: &bytes.Buffer{}, apprev: v1beta1.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-apprev2",
				Namespace: "dev2",
			},
			Spec: v1beta1.ApplicationRevisionSpec{},
			Status: v1beta1.ApplicationRevisionStatus{
				Workflow: &common2.WorkflowStatus{
					StartTime: metav1.Time{
						Time: ti,
					},
					Terminated: true,
				},
			},
		}, exp: tableOut("test-apprev2", "", "false", "", "2022-08-12 11:45:26", "Failed"),
		},
		"Executing or Failed": {out: &bytes.Buffer{}, apprev: v1beta1.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-apprev3",
				Namespace: "dev3",
			},
			Spec: v1beta1.ApplicationRevisionSpec{},
			Status: v1beta1.ApplicationRevisionStatus{
				Workflow: &common2.WorkflowStatus{
					StartTime: metav1.Time{
						Time: ti,
					},
				},
			},
		}, exp: tableOut("test-apprev3", "", "false", "", "2022-08-12 11:45:26", "Executing or Failed"),
		},
		"Compressed": {out: &bytes.Buffer{}, apprev: v1beta1.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-apprev3",
				Namespace: "dev3",
			},
			Spec: v1beta1.ApplicationRevisionSpec{
				Compression: v1beta1.ApplicationRevisionCompression{
					CompressedText: compression.CompressedText{
						Type: "zstd",
					},
				},
			},
			Status: v1beta1.ApplicationRevisionStatus{
				Workflow: &common2.WorkflowStatus{
					StartTime: metav1.Time{
						Time: ti,
					},
				},
			},
		}, exp: tableOut("test-apprev3", "", "false", "", "2022-08-12 11:45:26", "Executing or Failed"),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			printApprevs(tc.out, []v1beta1.ApplicationRevision{tc.apprev})
			assert.Contains(t, strings.ReplaceAll(tc.out.String(), " ", ""), strings.ReplaceAll(tc.out.String(), " ", ""))
			if tc.apprev.Spec.Compression.Type != compression.Uncompressed {
				assert.Contains(t, tc.out.String(), "Compressed")
			}
		})
	}
}

func tableOut(name, pv, s, hash, bt, status string) string {
	table := newUITable().AddRow("NAME", "PUBLISH_VERSION", "SUCCEEDED", "HASH", "BEGIN_TIME", "STATUS", "SIZE")
	table.AddRow(name, pv, s, hash, bt, status)

	return table.String()
}

func setupView() {
	viewContent, err := os.ReadFile("../../charts/vela-core/templates/velaql/application-revision.yaml")
	Expect(err).Should(BeNil())
	viewContent = bytes.ReplaceAll(viewContent, []byte("{{ include \"systemDefinitionNamespace\" . }}"), []byte(types.DefaultKubeVelaNS))
	cm := &v1.ConfigMap{}
	Expect(yaml.Unmarshal(viewContent, cm)).Should(BeNil())
	Expect(k8sClient.Create(context.TODO(), cm)).Should(BeNil())
}
