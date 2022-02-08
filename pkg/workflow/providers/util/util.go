/*
 Copyright 2022. The KubeVela Authors.

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

package util

import (
	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "util"
)

type provider struct{}

func (p *provider) PatchK8sObject(ctx wfContext.Context, v *value.Value, act types.Action) error {
	val, err := v.LookupValue("value")
	if err != nil {
		return err
	}
	pv, err := v.LookupValue("patch")
	if err != nil {
		return err
	}
	base, err := model.NewBase(val.CueValue())
	if err != nil {
		return err
	}
	patcher, err := model.NewOther(pv.CueValue())
	if err != nil {
		return err
	}
	if err = base.Unify(patcher); err != nil {
		return v.FillObject(err, "err")
	}

	workload, err := base.Unstructured()
	if err != nil {
		return v.FillObject(err, "err")
	}
	return v.FillObject(workload.Object, "result")
}

// String convert byte to string
func (p *provider) String(ctx wfContext.Context, v *value.Value, act types.Action) error {
	b, err := v.LookupValue("bt")
	if err != nil {
		return err
	}
	s, err := b.CueValue().Bytes()
	if err != nil {
		return err
	}
	return v.FillObject(string(s), "str")
}

// Install register handlers to provider discover.
func Install(p providers.Providers) {
	prd := &provider{}
	p.Register(ProviderName, map[string]providers.Handler{
		"patch-k8s-object": prd.PatchK8sObject,
		"string":           prd.String,
	})
}
