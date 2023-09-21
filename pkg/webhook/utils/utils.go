/*
 Copyright 2021. The KubeVela Authors.

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

package utils

import (
	"context"
	"regexp"
	"strings"

	"cuelang.org/go/cue/cuecontext"
	cueErrors "cuelang.org/go/cue/errors"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/core"
)

// ContextRegex to match '**: reference "context" not found'
var ContextRegex = `^.+:\sreference\s\"context\"\snot\sfound$`

// ValidateDefinitionRevision validate whether definition will modify the immutable object definitionRevision
func ValidateDefinitionRevision(ctx context.Context, cli client.Client, def runtime.Object, defRevNamespacedName types.NamespacedName) error {
	if errs := validation.IsQualifiedName(defRevNamespacedName.Name); len(errs) != 0 {
		return errors.Errorf("invalid definitionRevision name %s:%s", defRevNamespacedName.Name, strings.Join(errs, ","))
	}

	defRev := new(v1beta1.DefinitionRevision)
	if err := cli.Get(ctx, defRevNamespacedName, defRev); err != nil {
		return client.IgnoreNotFound(err)
	}

	newRev, _, err := core.GatherRevisionInfo(def)
	if err != nil {
		return err
	}
	if defRev.Spec.RevisionHash != newRev.Spec.RevisionHash {
		return errors.New("the definition's spec is different with existing definitionRevision's spec")
	}
	if !core.DeepEqualDefRevision(defRev, newRev) {
		return errors.New("the definition's spec is different with existing definitionRevision's spec")
	}
	return nil
}

// ValidateCueTemplate validate cueTemplate
func ValidateCueTemplate(cueTemplate string) error {

	val := cuecontext.New().CompileString(cueTemplate)
	if e := checkError(val.Err()); e != nil {
		return e
	}

	err := val.Validate()
	if e := checkError(err); e != nil {
		return e
	}
	return nil
}

func checkError(err error) error {
	re := regexp.MustCompile(ContextRegex)
	if err != nil {
		// ignore context not found error
		for _, e := range cueErrors.Errors(err) {
			if !re.MatchString(e.Error()) {
				return cueErrors.New(e.Error())
			}
		}
	}
	return nil
}
