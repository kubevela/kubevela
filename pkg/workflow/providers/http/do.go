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

package http

import (
	"context"
	"encoding/base64"
	"strings"

	"cuelang.org/go/cue"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/builtin"
	"github.com/oam-dev/kubevela/pkg/builtin/registry"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "http"
)

type provider struct {
	cli client.Client
	ns  string
}

// Do process http request.
func (h *provider) Do(ctx wfContext.Context, v *value.Value, act types.Action) error {
	tlsConfig, err := v.LookupValue("tls_config")
	if err == nil {
		secretName, err := tlsConfig.GetString("secret")
		if err != nil {
			return err
		}
		objectKey := client.ObjectKey{
			Namespace: h.ns,
			Name:      secretName,
		}
		index := strings.Index(secretName, "/")
		if index > 0 {
			objectKey.Namespace = secretName[:index-1]
			objectKey.Name = secretName[index:]
		}

		secret := new(v1.Secret)
		if err := h.cli.Get(context.Background(), objectKey, secret); err != nil {
			return err
		}
		if ca, ok := secret.Data["ca.crt"]; ok {
			caData, err := base64.StdEncoding.DecodeString(string(ca))
			if err != nil {
				return err
			}
			if err := v.FillObject(string(caData), "tls_config", "ca"); err != nil {
				return err
			}
		}
		if clientCert, ok := secret.Data["client.crt"]; ok {
			certData, err := base64.StdEncoding.DecodeString(string(clientCert))
			if err != nil {
				return err
			}
			if err := v.FillObject(string(certData), "tls_config", "client_crt"); err != nil {
				return err
			}
		}

		if clientKey, ok := secret.Data["client.key"]; ok {
			keyData, err := base64.StdEncoding.DecodeString(string(clientKey))
			if err != nil {
				return err
			}
			if err := v.FillObject(string(keyData), "tls_config", "client_key"); err != nil {
				return err
			}
		}
	}
	ret, err := builtin.RunTaskByKey("http", cue.Value{}, &registry.Meta{
		Obj: v.CueValue(),
	})
	if err != nil {
		return err
	}
	return v.FillObject(ret, "response")
}

// Install register handlers to provider discover.
func Install(p providers.Providers, cli client.Client, ns string) {
	prd := &provider{
		cli: cli,
		ns:  ns,
	}
	p.Register(ProviderName, map[string]providers.Handler{
		"do": prd.Do,
	})
}
