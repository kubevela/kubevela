/*
Copyright 2025 The KubeVela Authors.

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

package health

import (
	"encoding/json"
	"slices"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	CustomMessage  = "message"
	IsHealthPolicy = "isHealth"
)

type StatusRequest struct {
	Health    string
	Custom    string
	Details   string
	Parameter map[string]interface{}
}

type StatusResult struct {
	Healthy bool              `json:"healthy"`
	Message string            `json:"message,omitempty"`
	Details map[string]string `json:"details,omitempty"`
}

func CheckHealth(templateContext map[string]interface{}, healthPolicyTemplate string, parameter interface{}) (bool, error) {
	if healthPolicyTemplate == "" {
		return true, nil
	}
	runtimeContextBuff, err := formatRuntimeContext(templateContext, parameter)
	if err != nil {
		return false, err
	}
	var buff = healthPolicyTemplate + "\n" + runtimeContextBuff

	val := cuecontext.New().CompileString(buff)
	healthy, err := val.LookupPath(value.FieldPath(IsHealthPolicy)).Bool()
	if err != nil {
		return false, errors.WithMessage(err, "evaluate health status")
	}
	return healthy, nil
}

func GetStatus(templateContext map[string]interface{}, request *StatusRequest) (*StatusResult, error) {
	if templateContext["status"] == nil {
		templateContext["status"] = make(map[string]interface{})
	}

	templateContext, statusMap, mapErr := getStatusMap(templateContext, request.Details, request.Parameter)
	if mapErr != nil {
		klog.Warningf("failed to get status map: %v", mapErr)
	}

	healthy, healthErr := CheckHealth(templateContext, request.Health, request.Parameter)
	if healthErr != nil {
		klog.Warningf("failed to check health: %v", healthErr)
	}

	if statusMap, ok := templateContext["status"].(map[string]interface{}); ok {
		statusMap["healthy"] = healthy
	} else {
		klog.Warningf("templateContext['status'] is not a map[string]interface{}, cannot set healthy field")
	}

	message, msgErr := getStatusMessage(templateContext, request.Custom, request.Parameter)
	if msgErr != nil {
		klog.Warningf("failed to get status message: %v", msgErr)
	}

	return &StatusResult{
		Healthy: healthy,
		Message: message,
		Details: statusMap,
	}, nil
}

func getStatusMessage(templateContext map[string]interface{}, customStatusTemplate string, parameter interface{}) (string, error) {
	if customStatusTemplate == "" {
		return "", nil
	}
	runtimeContextBuff, err := formatRuntimeContext(templateContext, parameter)
	if err != nil {
		return "", err
	}
	var buff = customStatusTemplate + "\n" + runtimeContextBuff

	val := cuecontext.New().CompileString(buff)
	if val.Err() != nil {
		return "", errors.WithMessage(val.Err(), "compile status template")
	}
	message, err := val.LookupPath(value.FieldPath(CustomMessage)).String()
	if err != nil {
		return "", errors.WithMessage(err, "evaluate customStatus.message")
	}
	return message, nil
}

func getStatusMap(templateContext map[string]interface{}, statusFields string, parameter interface{}) (map[string]interface{}, map[string]string, error) {
	status := make(map[string]string)

	if templateContext["status"] == nil {
		templateContext["status"] = make(map[string]interface{})
	}

	if statusFields == "" {
		return templateContext, status, nil
	}

	runtimeContextBuff, err := formatRuntimeContext(templateContext, parameter)
	if err != nil {
		return templateContext, status, errors.WithMessage(err, "format runtime context")
	}
	cueCtx := cuecontext.New()

	var contextLabels []string
	contextVal := cueCtx.CompileString(runtimeContextBuff)
	iter, err := contextVal.Fields(cue.All())
	if err != nil {
		return templateContext, nil, errors.WithMessage(err, "get context fields")
	}
	for iter.Next() {
		contextLabels = append(contextLabels, util.GetIteratorLabel(*iter))
	}

	cueBuffer := runtimeContextBuff + "\n" + statusFields
	val := cueCtx.CompileString(cueBuffer)
	if val.Err() != nil {
		return templateContext, nil, errors.WithMessage(val.Err(), "compile status fields template")
	}
	iter, err = val.Fields()
	if err != nil {
		return templateContext, nil, errors.WithMessage(err, "get status fields")
	}

	detailsMap := make(map[string]interface{})

outer:
	for iter.Next() {
		label := util.GetIteratorLabel(*iter)

		if len(label) >= 32 {
			klog.Warningf("status.details field label %s is too long, skipping", label)
			continue // Skip labels that are too long
		}

		if slices.Contains(contextLabels, label) {
			continue // Skip fields that are already in the context
		}

		v := iter.Value()

		// Check if field should be excluded via attributes
		shouldExclude := false
		for _, a := range v.Attributes(cue.FieldAttr) {
			if a.Name() == "local" || a.Name() == "private" {
				shouldExclude = true
				break
			}
		}

		// For $ fields, include in context but not in status map
		if strings.HasPrefix(label, "$") {
			if err = v.Value().Validate(cue.Concrete(true)); err == nil {
				var nonStringValue interface{}
				if err := v.Value().Decode(&nonStringValue); err == nil {
					detailsMap[label] = nonStringValue
				}
			}
			continue // Skip adding to status map
		}

		// Skip excluded fields entirely
		if shouldExclude {
			continue outer
		}

		if err = v.Value().Validate(cue.Concrete(true)); err == nil {
			var nonStringValue interface{}
			if err := v.Value().Decode(&nonStringValue); err == nil {
				detailsMap[label] = nonStringValue
			}

			if v.Value().IncompleteKind() == cue.StringKind {
				status[label], _ = v.Value().String()
				continue
			}
			node := v.Value().Syntax(cue.Final())
			b, err := format.Node(node)
			if err != nil {
				return templateContext, nil, errors.WithMessagef(err, "format status field %s", label)
			}
			status[label] = string(b)
		} else {
			status[label] = cue.BottomKind.String() // Use a default value for invalid fields
		}
	}

	if statusContext, ok := templateContext["status"].(map[string]interface{}); ok {
		statusContext["details"] = detailsMap
	} else {
		klog.Warningf("templateContext['status'] is not a map[string]interface{}, cannot store details")
	}

	return templateContext, status, nil
}

func formatRuntimeContext(templateContext map[string]interface{}, parameter interface{}) (string, error) {
	var paramBuff = "parameter: {}\n"

	bt, err := json.Marshal(templateContext)
	if err != nil {
		return "", errors.WithMessage(err, "json marshal template context")
	}
	ctxBuff := "context: " + string(bt) + "\n"

	bt, err = json.Marshal(parameter)
	if err != nil {
		return "", errors.WithMessage(err, "json marshal template parameters")
	}
	if string(bt) != "null" {
		paramBuff = "parameter: " + string(bt) + "\n"
	}
	return ctxBuff + paramBuff, nil
}
