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

package usecase

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlattenKeyFunc(t *testing.T) {
	srcMap := map[string]interface{}{}
	err := json.Unmarshal([]byte(src), &srcMap)
	assert.NoError(t, err)

	dstMap := map[string]interface{}{}
	err = json.Unmarshal([]byte(dst), &dstMap)
	assert.NoError(t, err)

	res := map[string]interface{}{}
	flattenKey("", srcMap, res)
	assert.Equal(t, dstMap, res)
}

var (
	src = `{
    "OAMSpecVer":"v0.2",
    "admissionWebhooks":{
        "autoGenWorkloadDefinition":true,
        "certManager":{
            "enabled":false,
            "revisionHistoryLimit":3
        },
        "certificate":{
            "mountPath":"/etc/k8s-webhook-certs"
        },
        "enabled":true,
        "failurePolicy":"Fail",
        "patch":{
            "affinity":{

            },
            "enabled":true,
            "image":{
                "pullPolicy":"IfNotPresent",
                "repository":"oamdev/kube-webhook-certgen",
                "tag":"v2.3"
            },
            "tolerations":[

            ]
        }
    },
    "affinity":{

    },
    "applyOnceOnly":"off",
    "concurrentReconciles":4,
    "dependCheckWait":"30s",
    "disableCaps":"all",
    "fullnameOverride":"",
    "healthCheck":{
        "port":11440
    },
    "image":{
        "pullPolicy":"Always",
        "repository":"oamdev/vela-core",
        "tag":"v1.2.4"
    },
    "imagePullSecrets":[

    ],
    "imageRegistry":"",
    "logDebug":false,
    "logFileMaxSize":1024,
    "logFilePath":"",
    "nameOverride":"",
    "nodeSelector":{

    },
    "podSecurityContext":{

    },
    "rbac":{
        "create":true
    },
    "replicaCount":1,
    "resources":{
        "limits":{
            "cpu":"500m",
            "memory":"1Gi"
        },
        "requests":{
            "cpu":"50m",
            "memory":"20Mi"
        }
    },
    "securityContext":{

    },
    "serviceAccount":{
        "annotations":{

        },
        "create":true,
        "name":null
    },
    "systemDefinitionNamespace":"oam-runtime-system",
    "test":{
        "app":{
            "repository":"oamdev/busybox",
            "tag":"v1"
        }
    },
    "tolerations":[

    ],
    "webhookService":{
        "port":11443,
        "type":"ClusterIP"
    }
}`
	dst = `{
    "OAMSpecVer": "v0.2",
    "admissionWebhooks.autoGenWorkloadDefinition": true,
    "admissionWebhooks.certManager.enabled": false,
    "admissionWebhooks.certManager.revisionHistoryLimit": 3,
    "admissionWebhooks.certificate.mountPath": "/etc/k8s-webhook-certs",
    "admissionWebhooks.enabled": true,
    "admissionWebhooks.failurePolicy": "Fail",
    "admissionWebhooks.patch.enabled": true,
    "admissionWebhooks.patch.image.pullPolicy": "IfNotPresent",
    "admissionWebhooks.patch.image.repository": "oamdev/kube-webhook-certgen",
    "admissionWebhooks.patch.image.tag": "v2.3",
    "applyOnceOnly": "off",
    "concurrentReconciles": 4,
    "dependCheckWait": "30s",
    "disableCaps": "all",
    "fullnameOverride": "",
    "healthCheck.port": 11440,
    "image.pullPolicy": "Always",
    "image.repository": "oamdev/vela-core",
    "image.tag": "v1.2.4",
    "imageRegistry": "",
    "logDebug": false,
    "logFileMaxSize": 1024,
    "logFilePath": "",
    "nameOverride": "",
    "rbac.create": true,
    "replicaCount": 1,
    "resources.limits.cpu": "500m",
    "resources.limits.memory": "1Gi",
    "resources.requests.cpu": "50m",
    "resources.requests.memory": "20Mi",
    "serviceAccount.create": true,
    "serviceAccount.name": null,
    "systemDefinitionNamespace": "oam-runtime-system",
    "test.app.repository": "oamdev/busybox",
    "test.app.tag": "v1",
    "webhookService.port": 11443,
    "webhookService.type": "ClusterIP"
}`
)
